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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
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
		Name:  "test-tun-socket-device",
		Usage: "Test TUN device created from socket connection",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "socket-path",
				Usage: "Socket path to connect to",
				Value: "/tmp/hijack-tun.sock",
			},
			cli.IntFlag{
				Name:  "mtu",
				Usage: "MTU size for the device",
				Value: 1500,
			},
			cli.StringFlag{
				Name:  "test-ip",
				Usage: "Test IP address to add route and send traffic (default: 1.1.1.1)",
				Value: "1.1.1.1",
			},
			cli.StringFlag{
				Name:     "utun",
				Usage:    "Server-side TUN interface name (REQUIRED, e.g., utun12)",
				Required: true,
			},
		},
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
	socketPath := c.String("socket-path")
	mtu := c.Int("mtu")
	testIP := c.String("test-ip")
	tunInterface := c.String("utun")

	log.Infof("testing TUN device from socket: %s", socketPath)
	log.Infof("target TUN interface: %s", tunInterface)

	// 1. 从 socket 创建 Device
	device, err := lowtun.CreateDeviceFromSocket(socketPath, mtu)
	if err != nil {
		return utils.Errorf("failed to create device from socket: %v", err)
	}
	defer device.Close()

	// 2. 获取设备信息
	deviceMTU, err := device.MTU()
	if err != nil {
		return utils.Errorf("failed to get device MTU: %v", err)
	}
	log.Infof("device MTU: %d", deviceMTU)

	deviceName, err := device.Name()
	if err != nil {
		return utils.Errorf("failed to get device name: %v", err)
	}
	log.Infof("device name: %s (this is socket device, routes will be added to server-side TUN)", deviceName)

	batchSize := device.BatchSize()
	log.Infof("device batch size: %d", batchSize)

	// 3. 添加路由（如果指定了测试 IP）
	if testIP != "" {
		log.Infof("adding route for test IP: %s -> %s", testIP, tunInterface)
		if err := addRouteToInterface(testIP, tunInterface); err != nil {
			log.Warnf("failed to add route: %v", err)
			log.Infof("please manually add route with: sudo route add %s -interface %s", testIP, tunInterface)
		} else {
			log.Infof("✓ route added successfully: %s -> %s", testIP, tunInterface)
			defer func() {
				log.Infof("cleaning up route for %s", testIP)
				deleteRoute(testIP)
			}()
		}
	}

	// 4. 设置信号处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigChan
		log.Infof("received shutdown signal")
		cancel()
	}()

	// 6. 创建统计信息
	var (
		txBytes atomic.Uint64 // TUN -> Socket
		rxBytes atomic.Uint64 // Socket -> TUN
		txPkts  atomic.Uint64
		rxPkts  atomic.Uint64
	)

	// 7. 启动接收数据包的goroutine（监听从服务端转发来的数据包）
	go func() {
		log.Infof("starting packet receiver goroutine...")
		bufs := make([][]byte, batchSize)
		sizes := make([]int, batchSize)
		for i := range bufs {
			bufs[i] = make([]byte, deviceMTU+4)
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
				// 从设备读取数据包（服务端转发过来的）
				n, err := device.Read(bufs, sizes, 4)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					if err != io.EOF {
						log.Errorf("device read error: %v", err)
					}
					continue
				}

				// 处理接收到的数据包
				for i := 0; i < n; i++ {
					packet := bufs[i][4 : 4+sizes[i]]

					// 记录统计
					rxPkts.Add(1)
					rxBytes.Add(uint64(sizes[i]))

					// 打印数据包信息（仅前几个包）
					if rxPkts.Load() <= 10 {
						log.Infof("← RX packet #%d: %d bytes", rxPkts.Load(), sizes[i])
						if sizes[i] >= 20 && packet[0]>>4 == 4 {
							srcIP := net.IPv4(packet[12], packet[13], packet[14], packet[15])
							dstIP := net.IPv4(packet[16], packet[17], packet[18], packet[19])
							protocol := packet[9]
							log.Infof("  IPv4: %s -> %s, protocol: %d", srcIP, dstIP, protocol)
						}
					}
				}
			}
		}
	}()

	// 8. 监听 events
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-device.Events():
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

	// 9. 定期打印统计信息
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tx := txPkts.Load()
				rx := rxPkts.Load()
				txB := txBytes.Load()
				rxB := rxBytes.Load()
				log.Infof("Stats: TX %d pkts (%.2f KB), RX %d pkts (%.2f KB)",
					tx, float64(txB)/1024, rx, float64(rxB)/1024)
			}
		}
	}()

	log.Infof("device is ready, bidirectional forwarding started (press Ctrl+C to stop)...")

	// 10. 如果指定了测试 IP，执行自动化测试
	if testIP != "" {
		go func() {
			time.Sleep(2 * time.Second) // 等待路由生效

			log.Infof("\n========================================")
			log.Infof("Starting automated HTTP test to %s", testIP)
			log.Infof("Sending HTTP GET request...")
			log.Infof("========================================\n")

			// 记录测试前的流量统计
			txBefore := txPkts.Load()
			rxBefore := rxPkts.Load()
			log.Infof("Traffic before test - TX: %d pkts, RX: %d pkts", txBefore, rxBefore)

			// 使用 lowhttp.HTTP 发送 HTTP 请求
			log.Infof("Requesting: http://%s/", testIP)

			// 构建 HTTP GET 请求包
			packet := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", testIP)

			rsp, err := lowhttp.HTTP(
				lowhttp.WithPacketBytes([]byte(packet)),
				lowhttp.WithTimeout(5*time.Second),
			)
			if err != nil {
				log.Warnf("HTTP request failed: %v", err)
			} else {
				log.Infof("✓ HTTP request completed successfully")
				if rsp != nil && len(rsp.RawPacket) > 0 {
					log.Infof("Response: %d bytes received", len(rsp.RawPacket))
				}
			}

			// 等待一小段时间看统计
			time.Sleep(1 * time.Second)

			// 检查流量变化
			txAfter := txPkts.Load()
			rxAfter := rxPkts.Load()
			txDelta := txAfter - txBefore
			rxDelta := rxAfter - rxBefore

			log.Infof("\nTraffic after test:")
			log.Infof("  TX: %d pkts (Δ +%d), RX: %d pkts (Δ +%d)", txAfter, txDelta, rxAfter, rxDelta)

			if txDelta > 0 && rxDelta > 0 {
				log.Infof("\n✓✓✓ SUCCESS! Bidirectional traffic forwarding works! ✓✓✓")
				log.Infof("Both TX and RX traffic detected during test\n")
			} else if rxDelta > 0 && txDelta == 0 {
				log.Warnf("\n⚠ WARNING: Only RX traffic detected (%d packets), no TX traffic", rxDelta)
				log.Warnf("Packets received from server but not sent back. Check client forwarding.\n")
			} else if txDelta > 0 && rxDelta == 0 {
				log.Warnf("\n⚠ WARNING: Only TX traffic detected (%d packets), no RX traffic", txDelta)
				log.Warnf("Packets sent but no response received.\n")
			} else {
				log.Warnf("\n✗✗✗ WARNING: No traffic detected during test. ✗✗✗\n")
			}
		}()
	}

	// 11. 等待退出信号
	<-ctx.Done()
	log.Infof("shutting down...")

	// 打印最终统计
	log.Infof("\nFinal Statistics:")
	log.Infof("  TX: %d packets, %.2f KB", txPkts.Load(), float64(txBytes.Load())/1024)
	log.Infof("  RX: %d packets, %.2f KB", rxPkts.Load(), float64(rxBytes.Load())/1024)

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
