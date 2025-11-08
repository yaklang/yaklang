package yakcmds

import (
	"context"
	"encoding/binary"
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

	// 1. 创建 privileged device 并获取 socket path
	device, socketPath, err := lowtun.CreatePrivilegedDevice(1400)
	if err != nil {
		return utils.Errorf("failed to create privileged device: %v", err)
	}
	log.Infof("privileged device created successfully, socket path: %s", socketPath)

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

	// 4. 监控网络栈信息
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 获取路由表
				netStack := tvm.GetStack()
				routes := netStack.GetRouteTable()
				log.Infof("=== Network Stack Monitoring ===")
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

				// 获取网络栈统计信息
				stats := netStack.Stats()
				log.Infof("Stack statistics:")
				log.Infof("  IP: PacketsReceived=%d, PacketsSent=%d, OutgoingPacketErrors=%d, MalformedPacketsReceived=%d",
					stats.IP.PacketsReceived.Value(), stats.IP.PacketsSent.Value(), stats.IP.OutgoingPacketErrors.Value(), stats.IP.MalformedPacketsReceived.Value())
				log.Infof("  TCP: ActiveConnectionOpenings=%d, CurrentEstablished=%d, EstablishedResets=%d",
					stats.TCP.ActiveConnectionOpenings.Value(), stats.TCP.CurrentEstablished.Value(), stats.TCP.EstablishedResets.Value())
				log.Infof("  UDP: PacketsReceived=%d, PacketsSent=%d, ReceiveBufferErrors=%d",
					stats.UDP.PacketsReceived.Value(), stats.UDP.PacketsSent.Value(), stats.UDP.ReceiveBufferErrors.Value())
				log.Infof("===================================")
			}
		}
	}()

	// 5. 监听设备事件
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

	// 6. 等待退出信号
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
