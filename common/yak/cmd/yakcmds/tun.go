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

// testSocketConnection 测试是否可以用给定的密码连接到 socket
func testSocketConnection(socketPath, secret string) (bool, string, error) {
	// 尝试连接到 socket
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return false, "", err
	}
	defer conn.Close()

	// 如果需要认证，发送认证请求
	if secret != "" {
		authReq := map[string]string{"secret": secret}
		data, err := json.Marshal(authReq)
		if err != nil {
			return false, "", utils.Errorf("failed to marshal auth request: %v", err)
		}

		// 发送长度前缀和数据
		length := uint32(len(data))
		if err := binary.Write(conn, binary.BigEndian, length); err != nil {
			return false, "", utils.Errorf("failed to write auth length: %v", err)
		}
		if _, err := conn.Write(data); err != nil {
			return false, "", utils.Errorf("failed to write auth data: %v", err)
		}
	}

	// 读取响应
	var respLength uint32
	if err := binary.Read(conn, binary.BigEndian, &respLength); err != nil {
		return false, "", utils.Errorf("failed to read response length: %v", err)
	}

	if respLength > 1024*1024 {
		return false, "", utils.Errorf("response too large: %d bytes", respLength)
	}

	respData := make([]byte, respLength)
	if _, err := io.ReadFull(conn, respData); err != nil {
		return false, "", utils.Errorf("failed to read response: %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(respData, &resp); err != nil {
		return false, "", utils.Errorf("failed to unmarshal response: %v", err)
	}

	ok, _ := resp["ok"].(bool)
	if !ok {
		errMsg, _ := resp["error"].(string)
		return false, "", utils.Errorf("authentication failed: %s", errMsg)
	}

	utunName, _ := resp["utun"].(string)
	return true, utunName, nil
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

	// 0. 启动前先检查是否已有高权限进程在运行
	if _, err := os.Stat(socketPath); err == nil {
		log.Infof("found existing socket at %s, testing connection...", socketPath)
		if ok, utunName, err := testSocketConnection(socketPath, secret); ok {
			log.Infof("successfully connected to existing privileged process (utun: %s), exiting", utunName)
			os.Exit(0)
		} else {
			log.Warnf("socket exists but connection test failed: %v, will try to create new one", err)
			// 尝试删除旧的 socket 文件
			if err := os.Remove(socketPath); err != nil {
				log.Warnf("failed to remove stale socket: %v", err)
			}
		}
	}

	// 1. 首先创建 socket 监听器（快速失败）
	listener, err := lowtun.ListenSocket(socketPath)
	if err != nil {
		return utils.Errorf("failed to create socket listener: %v", err)
	}

	// 创建 PID lock 文件
	pidLockPath := socketPath + ".pid.lock"
	pid := os.Getpid()
	if err := os.WriteFile(pidLockPath, []byte(fmt.Sprintf("%d", pid)), 0644); err != nil {
		log.Warnf("failed to create PID lock file: %v", err)
	} else {
		log.Infof("created PID lock file: %s (PID: %d)", pidLockPath, pid)
	}

	defer func() {
		listener.Close()
		os.Remove(socketPath)
		os.Remove(pidLockPath)
		log.Infof("cleaned up socket file: %s", socketPath)
		log.Infof("cleaned up PID lock file: %s", pidLockPath)
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

	// 用于存储需要清理的路由
	var routesToCleanup []string

	defer func() {
		// 清理路由
		if len(routesToCleanup) > 0 {
			log.Infof("cleaning up %d routes", len(routesToCleanup))
			err := netutil.DeleteIPRoute(routesToCleanup)
			if err != nil {
				log.Errorf("failed to cleanup routes: %v", err)
			} else {
				log.Infof("routes cleaned up successfully")
			}
		}

		// 关闭设备
		log.Infof("closing TUN device: %s", tunName)
		device.Close()
		log.Infof("TUN device closed")
	}()
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

	// 4. 获取主机的公网网卡和IP地址（用于绑定源地址）
	log.Infof("getting host public network interface and IP...")
	hostInterface, hostIP, err := getPublicNetworkInterface()
	if err != nil {
		log.Errorf("failed to get public network interface: %v", err)
		return utils.Errorf("failed to get public network interface: %v", err)
	}
	log.Infof("host public interface: %s, IP: %s", hostInterface, hostIP)

	// 5. DNS 解析 yaklang.com 的所有 IP 并劫持
	log.Infof("resolving all IPs for yaklang.com...")
	exampleIPs, err := net.LookupIP("yaklang.com")
	if err != nil {
		log.Errorf("failed to resolve yaklang.com: %v", err)
		return utils.Errorf("failed to resolve yaklang.com: %v", err)
	}

	var exampleIPv4s []string
	for _, ip := range exampleIPs {
		ipStr := ip.String()
		// 只处理 IPv4 地址
		if strings.Contains(ipStr, ":") {
			continue
		}
		exampleIPv4s = append(exampleIPv4s, ipStr)
		log.Infof("resolved yaklang.com to: %s", ipStr)
	}

	if len(exampleIPv4s) == 0 {
		return utils.Errorf("failed to resolve yaklang.com to any IPv4 address")
	}

	// 6. 批量添加路由：1.1.1.1 和所有 yaklang.com 的 IP 都劫持到 TUN
	allIPs := append([]string{"1.1.1.1"}, exampleIPv4s...)
	log.Infof("batch adding routes for %d IPs to %s: %v", len(allIPs), tunName, allIPs)
	err = netutil.AddIPRouteToNetInterface(allIPs, tunName)
	if err != nil {
		log.Errorf("failed to batch add routes: %v", err)
		return utils.Errorf("failed to batch add routes: %v", err)
	}
	routesToCleanup = allIPs // 记录需要清理的路由
	log.Infof("successfully added %d routes to %s", len(allIPs), tunName)

	// 7. 设置 TCP 劫持处理器（中间人代理模式）
	// 生成两个 nonce：一个用于测试1，一个用于测试2
	nonce1 := fmt.Sprintf("NONCE1_%d_%s", time.Now().UnixNano(), utils.RandNumberStringBytes(16))
	nonce2 := fmt.Sprintf("NONCE2_%d_%s", time.Now().UnixNano(), utils.RandNumberStringBytes(16))
	log.Infof("generated test nonce1 (for 1.1.1.1 test): %s", nonce1)
	log.Infof("generated test nonce2 (for yaklang.com test): %s", nonce2)

	test1Passed := make(chan bool, 1)
	test2Passed := make(chan bool, 1)

	err = tvm.SetHijackTCPHandler(func(conn netstack.TCPConn) {
		remoteAddr := conn.RemoteAddr()
		localAddr := conn.LocalAddr()
		log.Infof("=== TCP Connection Hijacked ===")
		log.Infof("Remote: %s", remoteAddr)
		log.Infof("Local: %s", localAddr)
		log.Infof("===============================")

		// 读取客户端请求
		buf := make([]byte, 8192)
		n, err := conn.Read(buf)
		if err != nil {
			log.Errorf("failed to read from connection: %v", err)
			conn.Close()
			return
		}

		clientRequest := string(buf[:n])
		log.Infof("received client request (%d bytes):\n%s", n, clientRequest)

		// 提取请求中的 nonce（如果有）
		var extractedNonce string
		var testType string // "test1" or "test2"
		lines := strings.Split(clientRequest, "\r\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "X-Test-Nonce: ") {
				extractedNonce = strings.TrimPrefix(line, "X-Test-Nonce: ")
				if strings.HasPrefix(extractedNonce, "NONCE1_") {
					testType = "test1"
				} else if strings.HasPrefix(extractedNonce, "NONCE2_") {
					testType = "test2"
				}
				log.Infof("extracted nonce from request: %s (type: %s)", extractedNonce, testType)
				break
			}
		}

		// 判断是访问哪个目标（通过 localAddr 判断）
		localIPPort := localAddr.String()
		targetHost := ""
		targetPort := "80"

		if strings.HasPrefix(localIPPort, "1.1.1.1:") {
			// 测试1：1.1.1.1 → yaklang.com（需要绑定源地址，否则会再次被劫持）
			targetHost = "yaklang.com"
			log.Infof("[Test 1] Connection to 1.1.1.1, will forward to yaklang.com with source binding")
		} else {
			// 测试2：yaklang.com IP → yaklang.com（需要绑定源地址绕过路由表）
			for _, ip := range exampleIPv4s {
				if strings.HasPrefix(localIPPort, ip+":") {
					targetHost = ip
					log.Infof("[Test 2] Connection to yaklang.com IP %s, will forward with source binding", ip)
					break
				}
			}
		}

		if targetHost == "" {
			log.Errorf("unknown target host for local address: %s", localIPPort)
			conn.Close()
			return
		}

		// 连接到真实服务器（两个测试都使用源地址绑定，以绕过路由表劫持）
		log.Infof("connecting to %s:%s with source binding to %s...", targetHost, targetPort, hostIP)
		realConn, err := dialWithSourceIP(targetHost+":"+targetPort, hostIP, 5*time.Second)

		if err != nil {
			log.Errorf("failed to connect to real server: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 502 Bad Gateway\r\nX-Proxy-Nonce: %s\r\nConnection: close\r\n\r\nProxy Error: %v", extractedNonce, err)
			conn.Write([]byte(errorResponse))
			conn.Close()
			return
		}
		defer realConn.Close()

		// 转发客户端请求到真实服务器
		_, err = realConn.Write(buf[:n])
		if err != nil {
			log.Errorf("failed to forward request: %v", err)
			conn.Close()
			return
		}
		log.Infof("request forwarded to real server")

		// 读取真实服务器的响应
		realBuf := make([]byte, 8192)
		totalRead := 0
		realConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		for totalRead < len(realBuf) {
			n, err := realConn.Read(realBuf[totalRead:])
			if err != nil {
				if err != io.EOF {
					log.Errorf("error reading from real server: %v", err)
				}
				break
			}
			totalRead += n
			// 如果读到了完整的响应头，就可以开始处理
			if totalRead > 0 && strings.Contains(string(realBuf[:totalRead]), "\r\n\r\n") {
				break
			}
		}

		if totalRead == 0 {
			log.Errorf("no response from real server")
			conn.Close()
			return
		}

		realResponse := string(realBuf[:totalRead])
		log.Infof("received response from real server (%d bytes)", totalRead)

		// 修改响应，添加 X-Proxy-Nonce 头
		headerEndPos := strings.Index(realResponse, "\r\n\r\n")
		if headerEndPos == -1 {
			log.Errorf("invalid HTTP response from real server")
			conn.Close()
			return
		}

		// 在响应头末尾插入自定义头
		modifiedResponse := realResponse[:headerEndPos] +
			fmt.Sprintf("\r\nX-Proxy-Nonce: %s", extractedNonce) +
			realResponse[headerEndPos:]

		// 发送修改后的响应给客户端
		_, err = conn.Write([]byte(modifiedResponse))
		if err != nil {
			log.Errorf("failed to send response to client: %v", err)
		} else {
			log.Infof("sent proxied response with nonce header to client (%d bytes)", len(modifiedResponse))
		}

		// 通知对应测试通过
		if testType == "test1" {
			select {
			case test1Passed <- true:
			default:
			}
		} else if testType == "test2" {
			select {
			case test2Passed <- true:
			default:
			}
		}

		conn.Close()
	})
	if err != nil {
		return utils.Errorf("failed to set hijack TCP handler: %v", err)
	}
	log.Infof("TCP hijack handler (MITM proxy) set successfully")

	// 8. 启动测试1：访问 1.1.1.1:80（Host: yaklang.com）
	go func() {
		time.Sleep(2 * time.Second) // 等待路由生效

		log.Infof("===========================================")
		log.Infof("║ Starting Test 1: 1.1.1.1 → yaklang.com ║")
		log.Infof("===========================================")

		conn, err := net.DialTimeout("tcp", "1.1.1.1:80", 10*time.Second)
		if err != nil {
			log.Errorf("[Test 1] failed to dial 1.1.1.1:80: %v", err)
			return
		}
		defer conn.Close()

		httpRequest := fmt.Sprintf("GET / HTTP/1.1\r\nHost: yaklang.com\r\nX-Test-Nonce: %s\r\nConnection: close\r\n\r\n", nonce1)
		_, err = conn.Write([]byte(httpRequest))
		if err != nil {
			log.Errorf("[Test 1] failed to write request: %v", err)
			return
		}
		log.Infof("[Test 1] request sent with nonce: %s", nonce1)

		// 读取响应
		buf := make([]byte, 16384)
		totalRead := 0
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		for totalRead < len(buf) {
			n, err := conn.Read(buf[totalRead:])
			if err != nil {
				if err != io.EOF {
					log.Errorf("[Test 1] read error: %v", err)
				}
				break
			}
			totalRead += n
			if totalRead > 0 && strings.Contains(string(buf[:totalRead]), "\r\n\r\n") {
				time.Sleep(200 * time.Millisecond)
			}
		}

		if totalRead > 0 {
			response := string(buf[:totalRead])
			headerEndPos := strings.Index(response, "\r\n\r\n")
			if headerEndPos == -1 {
				log.Errorf("[Test 1] ✗ FAILED: Invalid HTTP response")
				return
			}

			headers := response[:headerEndPos]
			expectedHeader := fmt.Sprintf("X-Proxy-Nonce: %s", nonce1)
			if strings.Contains(headers, expectedHeader) {
				log.Infof("[Test 1] ✓ PASSED - Nonce verified!")
				select {
				case test1Passed <- true:
				default:
				}
			} else {
				log.Errorf("[Test 1] ✗ FAILED - Nonce not found in response")
			}
		} else {
			log.Errorf("[Test 1] ✗ FAILED: No response received")
		}
	}()

	// 9. 启动测试2：直接访问 yaklang.com（不绑定源地址，会被劫持）
	go func() {
		time.Sleep(3 * time.Second) // 稍晚启动，避免冲突

		log.Infof("===========================================")
		log.Infof("║ Starting Test 2: yaklang.com (hijacked) ║")
		log.Infof("===========================================")

		// 不绑定源地址，直接连接 yaklang.com → 会被劫持到 TUN
		conn, err := net.DialTimeout("tcp", "yaklang.com:80", 10*time.Second)
		if err != nil {
			log.Errorf("[Test 2] failed to dial yaklang.com:80: %v", err)
			return
		}
		defer conn.Close()

		httpRequest := fmt.Sprintf("GET / HTTP/1.1\r\nHost: yaklang.com\r\nX-Test-Nonce: %s\r\nConnection: close\r\n\r\n", nonce2)
		_, err = conn.Write([]byte(httpRequest))
		if err != nil {
			log.Errorf("[Test 2] failed to write request: %v", err)
			return
		}
		log.Infof("[Test 2] request sent with nonce: %s (will be hijacked)", nonce2)

		// 读取响应
		buf := make([]byte, 16384)
		totalRead := 0
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		for totalRead < len(buf) {
			n, err := conn.Read(buf[totalRead:])
			if err != nil {
				if err != io.EOF {
					log.Errorf("[Test 2] read error: %v", err)
				}
				break
			}
			totalRead += n
			if totalRead > 0 && strings.Contains(string(buf[:totalRead]), "\r\n\r\n") {
				time.Sleep(200 * time.Millisecond)
			}
		}

		if totalRead > 0 {
			response := string(buf[:totalRead])
			headerEndPos := strings.Index(response, "\r\n\r\n")
			if headerEndPos == -1 {
				log.Errorf("[Test 2] ✗ FAILED: Invalid HTTP response")
				return
			}

			headers := response[:headerEndPos]
			expectedHeader := fmt.Sprintf("X-Proxy-Nonce: %s", nonce2)
			if strings.Contains(headers, expectedHeader) {
				log.Infof("[Test 2] ✓ PASSED - Nonce verified! (Source binding worked)")
				select {
				case test2Passed <- true:
				default:
				}
			} else {
				log.Errorf("[Test 2] ✗ FAILED - Nonce not found in response")
			}
		} else {
			log.Errorf("[Test 2] ✗ FAILED: No response received")
		}
	}()

	// 10. 监听设备事件
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-tvm.GetTunnelDevice().Events():
				if !ok {
					return
				}
				switch event {
				case lowtun.EventUp:
					log.Infof("device event: UP")
				case lowtun.EventDown:
					log.Infof("device event: DOWN")
				case lowtun.EventMTUUpdate:
					log.Infof("device event: MTU UPDATE")
				}
			}
		}
	}()

	// 11. 等待两个测试都完成
	log.Infof("waiting for both tests to complete...")

	test1Done := false
	test2Done := false
	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-test1Passed:
			test1Done = true
			if test2Done {
				log.Infof("===========================================")
				log.Infof("║ ✓ ALL TESTS PASSED - SHUTTING DOWN... ║")
				log.Infof("===========================================")
				time.Sleep(500 * time.Millisecond)
				return nil
			}
		case <-test2Passed:
			test2Done = true
			if test1Done {
				log.Infof("===========================================")
				log.Infof("║ ✓ ALL TESTS PASSED - SHUTTING DOWN... ║")
				log.Infof("===========================================")
				time.Sleep(500 * time.Millisecond)
				return nil
			}
		case <-timeout:
			log.Errorf("===========================================")
			log.Errorf("║ ✗ TEST TIMEOUT AFTER 30s              ║")
			log.Errorf("===========================================")
			log.Errorf("  Test 1 (1.1.1.1): %v", test1Done)
			log.Errorf("  Test 2 (yaklang.com): %v", test2Done)
			return utils.Errorf("test timeout")
		case <-ctx.Done():
			log.Infof("test interrupted by user")
			return nil
		}
	}
}

// getPublicNetworkInterface 获取公网网卡和IP地址
func getPublicNetworkInterface() (interfaceName string, ipAddr string, err error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", "", err
	}

	// 优先查找非 lo、非 utun、非 bridge 的网卡
	for _, iface := range interfaces {
		// 跳过 loopback、down 状态、utun、bridge、docker 等虚拟网卡
		if iface.Flags&net.FlagLoopback != 0 ||
			iface.Flags&net.FlagUp == 0 ||
			strings.HasPrefix(iface.Name, "lo") ||
			strings.HasPrefix(iface.Name, "utun") ||
			strings.HasPrefix(iface.Name, "bridge") ||
			strings.HasPrefix(iface.Name, "docker") ||
			strings.HasPrefix(iface.Name, "veth") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			// 只处理 IPv4 地址，排除私有地址
			ip := ipNet.IP
			if ip.To4() == nil {
				continue
			}

			// 优先返回公网 IP，但私有 IP 也可以用于绑定
			// 私有地址：10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16
			ipStr := ip.String()
			isPrivate := ip.IsPrivate() || strings.HasPrefix(ipStr, "169.254.")

			// 如果找到非私有 IP，立即返回
			if !isPrivate {
				return iface.Name, ipStr, nil
			}

			// 记录私有 IP 作为备选
			if interfaceName == "" {
				interfaceName = iface.Name
				ipAddr = ipStr
			}
		}
	}

	if interfaceName != "" {
		return interfaceName, ipAddr, nil
	}

	return "", "", utils.Errorf("no suitable network interface found")
}

// dialWithSourceIP 使用指定的源 IP 地址连接目标服务器
func dialWithSourceIP(targetAddr string, sourceIP string, timeout time.Duration) (net.Conn, error) {
	// 解析目标地址
	targetHost, targetPort, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return nil, err
	}

	// 解析目标主机（可能是域名或 IP）
	targetIPs, err := net.LookupIP(targetHost)
	if err != nil {
		return nil, err
	}

	var targetIP net.IP
	for _, ip := range targetIPs {
		if ip.To4() != nil {
			targetIP = ip
			break
		}
	}

	if targetIP == nil {
		return nil, utils.Errorf("no IPv4 address found for %s", targetHost)
	}

	// 创建本地地址（绑定源 IP，端口由系统分配）
	localAddr := &net.TCPAddr{
		IP: net.ParseIP(sourceIP),
	}

	// 创建远程地址
	port, err := strconv.Atoi(targetPort)
	if err != nil {
		return nil, err
	}

	remoteAddr := &net.TCPAddr{
		IP:   targetIP,
		Port: port,
	}

	// 创建 dialer 并绑定本地地址
	dialer := &net.Dialer{
		LocalAddr: localAddr,
		Timeout:   timeout,
	}

	log.Infof("dialing %s from source %s", remoteAddr.String(), localAddr.String())
	return dialer.Dial("tcp", remoteAddr.String())
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
