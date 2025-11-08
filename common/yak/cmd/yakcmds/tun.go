package yakcmds

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun"
	"github.com/yaklang/yaklang/common/lowtun/netstack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv4"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv6"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
)

var TunCommands = []*cli.Command{
	{
		Name:  "forward-tun-to-socks",
		Usage: "Create a TUN device and forward traffic to unix socket",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "socket-path",
				Usage: "Unix socket path for forwarding traffic",
				Value: "/tmp/hijack-tun.sock",
			},
			cli.IntFlag{
				Name:  "mtu",
				Usage: "MTU size for TUN device",
				Value: 1500,
			},
			cli.StringFlag{
				Name:  "secret",
				Usage: "Secret for socket authentication (if set, clients must authenticate)",
				Value: "",
			},
		},
		Action: forwardTunToSocks,
	},
	{
		Name:   "test-tun-socket-device",
		Usage:  "Test TUN device created from socket connection",
		Flags:  []cli.Flag{},
		Action: testTunSocketDevice,
	},
}

func forwardTunToSocks(c *cli.Context) error {
	socketPath := c.String("socket-path")
	mtu := c.Int("mtu")
	secret := c.String("secret")

	if secret != "" {
		log.Infof("authentication enabled with secret: %s", secret)
	} else {
		log.Infof("authentication disabled (no secret provided)")
	}

	// 1. 首先创建 socket 监听器（快速失败）
	listener, err := lowtun.ListenSocket(socketPath)
	if err != nil {
		return utils.Errorf("failed to create socket listener: %v", err)
	}
	defer func() {
		listener.Close()
		os.Remove(socketPath)
		log.Infof("cleaned up socket file: %s", socketPath)
	}()
	log.Infof("socket listening on: %s", socketPath)

	// 2. 找到最大的 utun 编号
	maxUtunIndex, err := findMaxUtunIndex()
	if err != nil {
		return utils.Errorf("failed to find max utun index: %v", err)
	}

	nextUtunIndex := maxUtunIndex + 1
	tunName := fmt.Sprintf("utun%d", nextUtunIndex)
	log.Infof("creating TUN device: %s", tunName)

	// 3. 创建 TUN 设备
	tun, err := lowtun.CreateTUN(tunName, mtu)
	if err != nil {
		return utils.Errorf("failed to create TUN device: %v", err)
	}
	defer func() {
		log.Infof("closing TUN device")
		tun.Close()
	}()

	actualName, err := tun.Name()
	if err != nil {
		return utils.Errorf("failed to get TUN device name: %v", err)
	}
	log.Infof("TUN device created: %s", actualName)

	// 4. 分配不冲突的 IP 地址
	tunIP, err := findAvailableIP()
	if err != nil {
		return utils.Errorf("failed to find available IP: %v", err)
	}
	log.Infof("assigning IP %s to TUN device", tunIP)

	// 使用 ifconfig 配置 TUN 设备
	if err := configureTunDevice(actualName, tunIP, mtu); err != nil {
		return utils.Errorf("failed to configure TUN device: %v", err)
	}

	// 设置信号处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigChan
		log.Infof("received shutdown signal")
		cancel()
		listener.Close()
	}()

	// 将 TUN 设备转换为 ReadWriter
	tunRW, err := lowtun.ConvertTUNDeviceToReadWriter(tun, 4)
	if err != nil {
		return utils.Errorf("failed to convert TUN device to ReadWriter: %v", err)
	}

	// 管理多个客户端连接（fanout 机制）
	log.Infof("ready to accept client connections on socket: %s", socketPath)
	log.Infof("╔════════════════════════════════════════════════════════════════════╗")
	log.Infof("║ Client Usage:                                                      ║")
	log.Infof("║ yak test-tun-socket-device --utun %s \\              ║", fmt.Sprintf("%-25s", actualName))
	log.Infof("║                             --test-ip <ip> \\                       ║")
	log.Infof("║                             --socket-path %s       ║", fmt.Sprintf("%-19s", socketPath))
	log.Infof("╚════════════════════════════════════════════════════════════════════╝")

	type clientInfo struct {
		id     int
		conn   net.Conn
		writer *protocolWriter
		cancel context.CancelFunc
	}

	var (
		clientsMu sync.RWMutex
		clients   = make(map[int]*clientInfo)
		nextID    = 1
	)

	// TUN -> Socket (fanout 到所有客户端) - copyWriter
	go func() {
		buf := make([]byte, mtu)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// 从 TUN 读取一个数据包
				n, err := tunRW.Read(buf)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					if err != io.EOF {
						log.Errorf("TUN read error: %v", err)
					}
					continue
				}

				if n == 0 {
					continue
				}

				// 广播到所有客户端
				clientsMu.RLock()
				activeClients := make([]*clientInfo, 0, len(clients))
				for _, client := range clients {
					activeClients = append(activeClients, client)
				}
				clientsMu.RUnlock()

				if len(activeClients) == 0 {
					// 没有客户端，丢弃数据包
					continue
				}

				// 写入所有客户端
				for _, client := range activeClients {
					if _, err := client.writer.Write(buf[:n]); err != nil {
						log.Debugf("failed to write to client (id=%d): %v", client.id, err)
					} else {
						log.Debugf("forwarded packet from TUN to client (id=%d): %d bytes", client.id, n)
					}
				}
			}
		}
	}()

	// 持续接受新的客户端连接
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					log.Errorf("failed to accept connection: %v", err)
					continue
				}
			}

			// 发送初始响应（包含 utun 名称）
			// 如果设置了密码，先进行认证；否则直接发送欢迎消息
			if secret != "" {
				log.Infof("new connection, waiting for authentication...")
				if err := authenticateConnection(conn, secret, actualName); err != nil {
					log.Errorf("authentication failed: %v", err)
					conn.Close()
					continue
				}
				log.Infof("authentication successful")
			} else {
				// 未认证模式也需要发送初始响应
				if err := sendAuthResponse(conn, true, "", actualName); err != nil {
					log.Errorf("failed to send initial response: %v", err)
					conn.Close()
					continue
				}
				log.Infof("sent initial response to client")
			}

			clientsMu.Lock()
			clientID := nextID
			nextID++

			// 为每个客户端创建独立的 context
			clientCtx, clientCancel := context.WithCancel(ctx)

			client := &clientInfo{
				id:     clientID,
				conn:   conn,
				writer: &protocolWriter{conn: conn},
				cancel: clientCancel,
			}
			clients[clientID] = client
			clientsMu.Unlock()

			log.Infof("new client connected (id=%d), total clients: %d", clientID, len(clients))

			// Socket -> TUN (copyReader)
			go func(cli *clientInfo, cctx context.Context) {
				defer func() {
					log.Infof("client (id=%d) disconnected", cli.id)
					cli.conn.Close()

					clientsMu.Lock()
					delete(clients, cli.id)
					clientsMu.Unlock()
				}()

				// Socket 读取器（自动处理 4 字节长度前缀）
				socketReader := &protocolReader{conn: cli.conn, mtu: mtu}

				// 持续从 Socket 读取并写入 TUN
				_, err := io.Copy(tunRW, socketReader)
				if err != nil && err != io.EOF {
					log.Debugf("client (id=%d) Socket->TUN copy error: %v", cli.id, err)
				}
			}(client, clientCtx)
		}
	}()

	// 等待取消信号（服务端持续运行）
	<-ctx.Done()
	log.Infof("shutting down gracefully...")

	// 关闭所有客户端连接
	clientsMu.Lock()
	for _, client := range clients {
		client.cancel()
		client.conn.Close()
	}
	clientsMu.Unlock()

	return nil
}

// findMaxUtunIndex 找到当前系统中最大的 utun 索引号
func findMaxUtunIndex() (int, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return -1, err
	}

	maxIndex := -1
	for _, iface := range interfaces {
		if strings.HasPrefix(iface.Name, "utun") {
			indexStr := strings.TrimPrefix(iface.Name, "utun")
			if index, err := strconv.Atoi(indexStr); err == nil {
				if index > maxIndex {
					maxIndex = index
				}
			}
		}
	}

	return maxIndex, nil
}

// findAvailableIP 找到一个不冲突的内网 IP 地址
func findAvailableIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	// 收集所有已使用的 IP 地址
	usedIPs := make(map[string]bool)
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if ok {
				usedIPs[ipNet.IP.String()] = true
			}
		}
	}

	// 尝试在 10.16.0.0/16 网段中找到可用的 IP
	// 尝试 10.16.0.1 到 10.16.255.254
	for i := 0; i < 256; i++ {
		for j := 1; j < 255; j++ {
			candidate := fmt.Sprintf("10.16.%d.%d", i, j)
			if !usedIPs[candidate] {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("no available IP address found in 10.16.0.0/16 range")
}

// configureTunDevice 使用 ifconfig 配置 TUN 设备
func configureTunDevice(tunName, tunIP string, mtu int) error {
	// 在 macOS 上，utun 设备需要设置点对点地址
	// 使用 tunIP 作为本地地址，tunIP 的下一个地址作为远端地址
	ipParts := strings.Split(tunIP, ".")
	if len(ipParts) != 4 {
		return fmt.Errorf("invalid IP address: %s", tunIP)
	}

	lastOctet, err := strconv.Atoi(ipParts[3])
	if err != nil {
		return fmt.Errorf("invalid IP address: %s", tunIP)
	}

	// 远端地址为本地地址 +1
	remoteIP := fmt.Sprintf("%s.%s.%s.%d", ipParts[0], ipParts[1], ipParts[2], lastOctet+1)

	// 配置 IP 地址 (点对点模式)
	cmd := exec.Command("ifconfig", tunName, tunIP, remoteIP, "up")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to configure IP address: %v, output: %s", err, string(output))
	}
	log.Infof("configured TUN device with IP: %s -> %s", tunIP, remoteIP)

	// 设置 MTU
	cmd = exec.Command("ifconfig", tunName, "mtu", strconv.Itoa(mtu))
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set MTU: %v, output: %s", err, string(output))
	}
	log.Infof("set MTU to %d", mtu)

	// 验证设备状态
	cmd = exec.Command("ifconfig", tunName)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to verify device status: %v", err)
	}
	log.Infof("TUN device status:\n%s", string(output))

	return nil
}

func testTunSocketDevice(c *cli.Context) error {
	log.Infof("creating privileged TUN device with MTU 1400")

	// 1. 创建 privileged device 并获取 TUN 设备名称
	device, tunName, err := lowtun.CreatePrivilegedDevice(1400)
	if err != nil {
		return utils.Errorf("failed to create privileged device: %v", err)
	}
	log.Infof("privileged device created successfully, TUN device name: %s", tunName)

	// 2. 设置信号处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigChan
		log.Infof("received shutdown signal")
		cancel()
	}()

	// 3. 使用 NewTunVirtualMachineFromDevice 创建网络栈虚拟机
	log.Infof("creating network stack virtual machine from device...")
	tvm, err := netstackvm.NewTunVirtualMachineFromDevice(ctx, device)
	if err != nil {
		return utils.Errorf("failed to create TUN virtual machine from device: %v", err)
	}
	defer tvm.Close()

	log.Infof("network stack virtual machine created successfully")
	log.Infof("tunnel name: %s", tvm.GetTunnelName())

	// 4. 添加路由规则，将 1.1.1.1 劫持到 TUN 设备
	log.Infof("adding route for 1.1.1.1 to %s", tunName)
	err = netutil.AddIPRouteToNetInterface("1.1.1.1", tunName)
	if err != nil {
		log.Errorf("failed to add route: %v", err)
		return utils.Errorf("failed to add route: %v", err)
	}
	log.Infof("route added successfully")

	// 5. 设置 TCP 劫持处理器
	tcpConnReceived := make(chan bool, 1)
	err = tvm.SetHijackTCPHandler(func(conn netstack.TCPConn) {
		remoteAddr := conn.RemoteAddr()
		localAddr := conn.LocalAddr()
		log.Infof("=== TCP Connection Hijacked ===")
		log.Infof("Remote: %s", remoteAddr)
		log.Infof("Local: %s", localAddr)
		log.Infof("===============================")

		// 通知测试收到了连接
		select {
		case tcpConnReceived <- true:
		default:
		}

		// 读取一些数据
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			log.Errorf("failed to read from connection: %v", err)
		} else {
			log.Infof("received %d bytes from connection: %s", n, string(buf[:n]))
		}

		// 关闭连接
		conn.Close()
	})
	if err != nil {
		return utils.Errorf("failed to set hijack TCP handler: %v", err)
	}
	log.Infof("TCP hijack handler set successfully")

	// 6. 发送测试数据包到 1.1.1.1:80
	go func() {
		// 等待一下，确保路由生效
		time.Sleep(2 * time.Second)

		log.Infof("sending test TCP packet to 1.1.1.1:80")
		conn, err := net.DialTimeout("tcp", "1.1.1.1:80", 5*time.Second)
		if err != nil {
			log.Errorf("failed to dial 1.1.1.1:80: %v", err)
			return
		}
		defer conn.Close()

		// 发送 HTTP GET 请求
		httpRequest := "GET / HTTP/1.1\r\nHost: 1.1.1.1\r\nConnection: close\r\n\r\n"
		_, err = conn.Write([]byte(httpRequest))
		if err != nil {
			log.Errorf("failed to write to connection: %v", err)
			return
		}
		log.Infof("test packet sent successfully")

		// 等待响应
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			log.Infof("read from connection completed (expected): %v", err)
		} else {
			log.Infof("received response (%d bytes): %s", n, string(buf[:n]))
		}
	}()

	// 7. 等待接收到 TCP 连接
	go func() {
		select {
		case <-tcpConnReceived:
			log.Infof("✓ Test PASSED: TCP connection successfully hijacked!")
		case <-time.After(10 * time.Second):
			log.Errorf("✗ Test FAILED: No TCP connection received within 10 seconds")
		case <-ctx.Done():
			return
		}
	}()

	// 8. 监控网络栈信息（仅在数据变化时显示）
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		// 保存上次的统计数据
		var lastIPPacketsReceived, lastIPPacketsSent uint64
		var lastTCPActiveOpens, lastTCPEstablished uint64
		var lastUDPPacketsReceived, lastUDPPacketsSent uint64

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 获取网络栈统计信息
				netStack := tvm.GetStack()
				stats := netStack.Stats()

				// 当前统计数据
				currentIPPacketsReceived := stats.IP.PacketsReceived.Value()
				currentIPPacketsSent := stats.IP.PacketsSent.Value()
				currentTCPActiveOpens := stats.TCP.ActiveConnectionOpenings.Value()
				currentTCPEstablished := stats.TCP.CurrentEstablished.Value()
				currentUDPPacketsReceived := stats.UDP.PacketsReceived.Value()
				currentUDPPacketsSent := stats.UDP.PacketsSent.Value()

				// 检查是否有变化
				hasChange := currentIPPacketsReceived != lastIPPacketsReceived ||
					currentIPPacketsSent != lastIPPacketsSent ||
					currentTCPActiveOpens != lastTCPActiveOpens ||
					currentTCPEstablished != lastTCPEstablished ||
					currentUDPPacketsReceived != lastUDPPacketsReceived ||
					currentUDPPacketsSent != lastUDPPacketsSent

				// 只在有变化时显示
				if hasChange {
					log.Infof("=== Network Stack Monitoring (Data Changed) ===")

					// 获取路由表
					routes := netStack.GetRouteTable()
					log.Infof("Route table (%d routes):", len(routes))
					for i, route := range routes {
						log.Infof("  Route %d: NIC=%d, Destination=%s, Gateway=%s, MTU=%d",
							i+1, route.NIC, route.Destination, route.Gateway, route.MTU)
					}

					// 获取 NIC 信息
					nicInfo := netStack.NICInfo()
					log.Infof("NIC info (%d NICs):", len(nicInfo))
					for nicID, info := range nicInfo {
						log.Infof("  NIC %d:", nicID)
						log.Infof("    Name: %s", info.Name)
						log.Infof("    LinkAddress: %s", info.LinkAddress)
						log.Infof("    Flags: %v", info.Flags)
						log.Infof("    MTU: %d", info.MTU)
						log.Infof("    Protocol addresses (%d):", len(info.ProtocolAddresses))
						for _, addr := range info.ProtocolAddresses {
							log.Infof("      %s/%d", addr.AddressWithPrefix.Address, addr.AddressWithPrefix.PrefixLen)
						}
					}

					// 显示统计信息
					log.Infof("Stack statistics:")
					log.Infof("  IP: PacketsReceived=%d, PacketsSent=%d, OutgoingPacketErrors=%d, MalformedPacketsReceived=%d",
						currentIPPacketsReceived, currentIPPacketsSent, stats.IP.OutgoingPacketErrors.Value(), stats.IP.MalformedPacketsReceived.Value())
					log.Infof("  TCP: ActiveConnectionOpenings=%d, CurrentEstablished=%d, EstablishedResets=%d",
						currentTCPActiveOpens, currentTCPEstablished, stats.TCP.EstablishedResets.Value())
					log.Infof("  UDP: PacketsReceived=%d, PacketsSent=%d, ReceiveBufferErrors=%d",
						currentUDPPacketsReceived, currentUDPPacketsSent, stats.UDP.ReceiveBufferErrors.Value())
					log.Infof("===================================")

					// 更新上次的统计数据
					lastIPPacketsReceived = currentIPPacketsReceived
					lastIPPacketsSent = currentIPPacketsSent
					lastTCPActiveOpens = currentTCPActiveOpens
					lastTCPEstablished = currentTCPEstablished
					lastUDPPacketsReceived = currentUDPPacketsReceived
					lastUDPPacketsSent = currentUDPPacketsSent
				}
			}
		}
	}()

	// 9. 监听设备事件
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-tvm.GetTunnelDevice().Events():
				if !ok {
					log.Infof("device events channel closed")
					return
				}
				switch event {
				case lowtun.EventUp:
					log.Infof("device event: UP")
				case lowtun.EventDown:
					log.Infof("device event: DOWN")
				case lowtun.EventMTUUpdate:
					log.Infof("device event: MTU UPDATE")
				default:
					log.Infof("device event: UNKNOWN (%d)", event)
				}
			}
		}
	}()

	log.Infof("network stack virtual machine is ready (press Ctrl+C to stop)...")

	// 10. 等待退出信号
	<-ctx.Done()
	log.Infof("shutting down network stack virtual machine...")

	return nil
}

// initNetStackForTun 初始化网络栈（复制自 netstackvm.defaultInitNetStack）
func initNetStackForTun(s *stack.Stack) error {
	// 设置默认 TTL
	opt := tcpip.DefaultTTLOption(64)
	if err := s.SetNetworkProtocolOption(ipv4.ProtocolNumber, &opt); err != nil {
		return utils.Errorf("set ipv4 default TTL: %s", err)
	}
	if err := s.SetNetworkProtocolOption(ipv6.ProtocolNumber, &opt); err != nil {
		return utils.Errorf("set ipv6 default TTL: %s", err)
	}

	// 启用转发
	if err := s.SetForwardingDefaultAndAllNICs(ipv4.ProtocolNumber, true); err != nil {
		return utils.Errorf("set ipv4 forwarding: %s", err)
	}
	if err := s.SetForwardingDefaultAndAllNICs(ipv6.ProtocolNumber, true); err != nil {
		return utils.Errorf("set ipv6 forwarding: %s", err)
	}

	// 设置 ICMP 限制
	s.SetICMPBurst(50)
	s.SetICMPLimit(1000)

	// 设置 TCP 发送缓冲区大小范围
	sndOpt := tcpip.TCPSendBufferSizeRangeOption{
		Min:     tcp.MinBufferSize,
		Default: tcp.DefaultSendBufferSize,
		Max:     tcp.MaxBufferSize,
	}
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &sndOpt); err != nil {
		return utils.Errorf("set TCP send buffer size range: %s", err)
	}

	// 设置 TCP 接收缓冲区大小范围
	rcvOpt := tcpip.TCPReceiveBufferSizeRangeOption{
		Min:     tcp.MinBufferSize,
		Default: tcp.DefaultReceiveBufferSize,
		Max:     tcp.MaxBufferSize,
	}
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &rcvOpt); err != nil {
		return utils.Errorf("set TCP receive buffer size range: %s", err)
	}

	// 设置 TCP 拥塞控制算法
	tcpOpt := tcpip.CongestionControlOption("reno")
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &tcpOpt); err != nil {
		return utils.Errorf("set TCP congestion control algorithm: %s", err)
	}

	// 禁用 TCP delay (Nagle's algorithm)
	delayOpt := tcpip.TCPDelayEnabled(false)
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &delayOpt); err != nil {
		return utils.Errorf("set TCP delay enabled: %s", err)
	}

	// 禁用 TCP 接收缓冲区自动调整
	tcpModerateReceiveBufferOpt := tcpip.TCPModerateReceiveBufferOption(false)
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &tcpModerateReceiveBufferOpt); err != nil {
		return utils.Errorf("set TCP moderate receive buffer: %s", err)
	}

	// 启用 TCP SACK
	sackOpt := tcpip.TCPSACKEnabled(true)
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &sackOpt); err != nil {
		return utils.Errorf("set TCP SACK enabled: %s", err)
	}

	// 设置 TCP 恢复算法
	recoveryOpt := tcpip.TCPRACKLossDetection
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &recoveryOpt); err != nil {
		return utils.Errorf("set TCP Recovery: %s", err)
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// addRouteToInterface 添加路由到指定接口
func addRouteToInterface(ip, interfaceName string) error {
	return netutil.AddSpecificIPRouteToNetInterface(ip, interfaceName)
}

// deleteRoute 删除指定 IP 的路由
func deleteRoute(ip string) error {
	err := netutil.DeleteSpecificIPRoute(ip)
	if err != nil {
		log.Warnf("failed to delete route for %s: %v", ip, err)
	}
	return err
}

// authenticateConnection 服务器端认证：读取 {"secret": "..."} 并验证，然后回复 {"ok": true, "utun": "..."} 或 {"ok": false, "error": "..."}
func authenticateConnection(conn net.Conn, expectedSecret string, tunName string) error {
	// 1. 读取认证请求
	var lengthBuf [4]byte
	if _, err := io.ReadFull(conn, lengthBuf[:]); err != nil {
		return utils.Errorf("failed to read auth request length: %v", err)
	}

	reqLen := int(binary.BigEndian.Uint32(lengthBuf[:]))
	if reqLen <= 0 || reqLen > 1024 {
		return utils.Errorf("invalid auth request length: %d", reqLen)
	}

	reqData := make([]byte, reqLen)
	if _, err := io.ReadFull(conn, reqData); err != nil {
		return utils.Errorf("failed to read auth request: %v", err)
	}

	log.Debugf("received auth request: %s", string(reqData))

	// 2. 解析认证请求
	var authReq map[string]string
	if err := json.Unmarshal(reqData, &authReq); err != nil {
		sendAuthResponse(conn, false, "invalid auth request format", tunName)
		return utils.Errorf("failed to unmarshal auth request: %v", err)
	}

	// 3. 验证密码
	clientSecret, exists := authReq["secret"]
	if !exists {
		sendAuthResponse(conn, false, "missing secret field", tunName)
		return utils.Errorf("missing secret field in auth request")
	}

	if clientSecret != expectedSecret {
		sendAuthResponse(conn, false, "invalid secret", tunName)
		return utils.Errorf("invalid secret: expected %s, got %s", expectedSecret, clientSecret)
	}

	// 4. 认证成功，发送响应
	if err := sendAuthResponse(conn, true, "", tunName); err != nil {
		return utils.Errorf("failed to send auth response: %v", err)
	}

	log.Debugf("authentication successful")
	return nil
}

// sendAuthResponse 发送认证响应，包含 utun 名称
func sendAuthResponse(conn net.Conn, ok bool, errMsg string, tunName string) error {
	resp := map[string]interface{}{
		"ok":   ok,
		"utun": tunName,
	}
	if errMsg != "" {
		resp["error"] = errMsg
	}

	respData, err := json.Marshal(resp)
	if err != nil {
		return utils.Errorf("failed to marshal auth response: %v", err)
	}

	// 写入长度前缀
	var lengthBuf [4]byte
	binary.BigEndian.PutUint32(lengthBuf[:], uint32(len(respData)))
	if _, err := conn.Write(lengthBuf[:]); err != nil {
		return utils.Errorf("failed to write auth response length: %v", err)
	}

	// 写入响应数据
	if _, err := conn.Write(respData); err != nil {
		return utils.Errorf("failed to write auth response: %v", err)
	}

	log.Debugf("sent auth response: %s", string(respData))
	return nil
}

// protocolReader 实现 io.Reader，自动处理 4 字节长度前缀
type protocolReader struct {
	conn net.Conn
	mtu  int
}

func (r *protocolReader) Read(p []byte) (n int, err error) {
	// 读取 4 字节长度头
	var lengthBuf [4]byte
	if _, err := io.ReadFull(r.conn, lengthBuf[:]); err != nil {
		return 0, err
	}

	// 解析长度
	packetLen := int(binary.BigEndian.Uint32(lengthBuf[:]))
	if packetLen <= 0 || packetLen > r.mtu {
		return 0, utils.Errorf("invalid packet length: %d", packetLen)
	}

	if packetLen > len(p) {
		return 0, utils.Errorf("buffer too small: need %d, have %d", packetLen, len(p))
	}

	// 读取数据包内容
	return io.ReadFull(r.conn, p[:packetLen])
}

// protocolWriter 实现 io.Writer，自动添加 4 字节长度前缀
type protocolWriter struct {
	conn net.Conn
}

func (w *protocolWriter) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	// 写入 4 字节长度头
	var lengthBuf [4]byte
	binary.BigEndian.PutUint32(lengthBuf[:], uint32(len(p)))
	if _, err := w.conn.Write(lengthBuf[:]); err != nil {
		return 0, err
	}

	// 写入数据包内容
	return w.conn.Write(p)
}
