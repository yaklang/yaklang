package yakcmds

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun"
	"github.com/yaklang/yaklang/common/minimartian"
	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
)

var TunCommands = []*cli.Command{
	{
		Name:  "modify-route-to-socks",
		Usage: "Create a route manager process server listen to unix socket",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "socket-path",
				Usage: "Unix socket path for forwarding traffic",
				Value: "/tmp/route.sock",
			},
			cli.StringFlag{
				Name:  "secret",
				Usage: "Unix socket path (if set, clients must authenticate)",
				Value: "",
			},
		},
		Action: routeManagerToSocks,
	},
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
	{
		Name:   "reset-forward-tun-to-socks-password",
		Usage:  "Reset the password for forward-tun-to-socks privileged process (for testing authentication failure)",
		Flags:  []cli.Flag{},
		Action: resetForwardTunPassword,
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
	if runtime.GOOS == "windows" {
		return configureTunDeviceWin(tunName, tunIP, mtu)
	} else {
		return configureTunDeviceOther(tunName, tunIP, mtu)
	}
}

func configureTunDeviceOther(tunName, tunIP string, mtu int) error {
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

func configureTunDeviceWin(tunName, tunIP string, mtu int) error {
	// 在 Windows 上，我们为接口设置 IP 和子网掩码。
	// 对于 VPN 隧道，一个常见的子网掩码是 /24 (255.255.255.0) 或 /32 (255.255.255.255)。
	// 这里我们使用 /24 作为示例，你可以根据需要调整。
	subnetMask := "255.255.255.0"
	log.Printf("Configuring TUN device '%s' on Windows...", tunName)
	// 1. 配置 IP 地址和子网掩码
	// 命令: netsh interface ip set address name="tunName" static tunIP subnetMask
	// netsh 命令需要管理员权限才能执行
	cmd := exec.Command("netsh", "interface", "ip", "set", "address",
		fmt.Sprintf("name=%q", tunName), // 使用 %q 来为接口名称加上引号，防止名称中包含空格
		"static",
		tunIP,
		subnetMask,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 检查输出中是否包含 "Run as administrator" 错误
		if strings.Contains(string(output), "administrator") {
			return fmt.Errorf("permission denied. please run your application as an administrator to configure the network interface. details: %v, output: %s", err, string(output))
		}
		return fmt.Errorf("failed to configure IP address: %v, output: %s", err, string(output))
	}
	log.Printf("Configured TUN device with IP: %s, Subnet Mask: %s", tunIP, subnetMask)
	// 2. 设置 MTU
	// 命令: netsh interface ipv4 set subinterface "tunName" mtu=mtu store=persistent
	cmd = exec.Command("netsh", "interface", "ipv4", "set", "subinterface",
		fmt.Sprintf("interface=%q", tunName),
		fmt.Sprintf("mtu=%d", mtu),
		"store=persistent", // 使设置在重启后依然有效
	)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set MTU: %v, output: %s", err, string(output))
	}
	log.Printf("Set MTU to %d", mtu)
	// 3. 验证设备状态 (可选，但推荐)
	// 命令: netsh interface ip show config name="tunName"
	cmd = exec.Command("netsh", "interface", "ip", "show", "config", fmt.Sprintf("name=%q", tunName))
	output, err = cmd.CombinedOutput()
	if err != nil {
		// 即使验证失败，配置可能也已成功，所以只记录警告
		log.Printf("Warning: failed to verify device status: %v, output: %s", err, string(output))
	} else {
		log.Printf("TUN device status:\n%s", string(output))
	}
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

	// 4. 获取主机的公网网卡和IP地址（用于绑定源地址和强主机模式）
	log.Infof("getting host public network interface and IP...")
	hostInterface, hostIP, err := GetPublicNetworkInterface()
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

	// 7. 创建 channel 用于接收 TUN 的连接
	connChan := make(chan net.Conn, 1000)

	// 使用 StartToMergeTCPConnectionChannel 将 TUN 连接发送到 channel
	log.Infof("starting to merge TCP connection channel...")
	err = tvm.StartToMergeTCPConnectionChannel(ctx, connChan)
	if err != nil {
		return utils.Errorf("failed to start merge TCP connection channel: %v", err)
	}
	log.Infof("TCP connection channel merge started successfully")

	// 8. 创建 WrapperedConn channel，使用强主机模式避免环回
	// 将 net.Conn 转换为 *minimartian.WrapperedConn，并设置强主机模式的本地地址为公网 IP
	wrappedConnChan := make(chan *minimartian.WrapperedConn, 1000)
	go func() {
		defer close(wrappedConnChan)
		for conn := range connChan {
			// 使用 NewWrapperedConnWithStrongLocalHost 创建带有强主机模式的连接
			// 设置公网 IP 作为本地地址，这样 MITM 发送数据包时会绑定到这个 IP，避免环回
			wrapped := minimartian.NewWrapperedConnWithStrongLocalHost(conn, hostIP, nil)
			wrappedConnChan <- wrapped
		}
	}()

	// 9. 创建 crep MITM 服务器，使用 ExtraIncomingConnection Ex 接口（支持强主机模式）
	// 生成测试 nonce 用于验证劫持是否生效
	testNonce := fmt.Sprintf("TUN_MITM_TEST_%d_%s", time.Now().UnixNano(), utils.RandNumberStringBytes(16))
	log.Infof("generated test nonce: %s", testNonce)
	log.Infof("using strong host mode with local address: %s", hostIP)

	testPassed := make(chan bool, 1)
	hijackedURLs := make(map[string]bool)
	hijackedURLsMu := sync.Mutex{}

	// 创建 MITM 服务器，使用支持强主机模式的 EX 接口
	mitmServer, err := crep.NewMITMServer(
		crep.MITM_SetExtraIncomingConnectionChannel(wrappedConnChan),
		// crep.MITM_SetTunMode(true),
		crep.MITM_SetHTTPResponseHijackRaw(func(isHttps bool, req *http.Request, rspIns *http.Response, rsp []byte, remoteAddr string) []byte {
			// 验证劫持是否生效：在响应中添加测试 nonce
			urlStr := ""
			if req != nil && req.URL != nil {
				urlStr = req.URL.String()
			}

			hijackedURLsMu.Lock()
			hijackedURLs[urlStr] = true
			hijackedURLsMu.Unlock()

			log.Infof("[MITM] Hijacked response for URL: %s (HTTPS: %v)", urlStr, isHttps)

			// 解析响应以获取状态码和 header
			rspParsed, err := utils.ReadHTTPResponseFromBytes(rsp, req)
			if err != nil {
				log.Errorf("[MITM] Failed to parse response: %v", err)
				return rsp
			}

			// 打印完整的响应头（状态行 + 所有 header）
			log.Infof("[MITM] ==========================================")
			log.Infof("[MITM] Response Status: %s", rspParsed.Status)
			log.Infof("[MITM] Response Headers:")
			for key, values := range rspParsed.Header {
				for _, value := range values {
					log.Infof("[MITM]   %s: %s", key, value)
				}
			}
			log.Infof("[MITM] ==========================================")

			// 检查状态码，如果不是 200，记录警告
			if rspParsed.StatusCode != 200 {
				log.Warnf("[MITM] Response status code is %d, expected 200 OK", rspParsed.StatusCode)
			}

			// 检查是否是 yaklang.com 的请求
			if strings.Contains(urlStr, "yaklang.com") {
				// 在响应头中添加测试 nonce
				rspStr := string(rsp)
				headerEndPos := strings.Index(rspStr, "\r\n\r\n")
				if headerEndPos != -1 {
					// 检查是否已经包含 nonce header，避免重复添加
					headerPart := rspStr[:headerEndPos]
					if !strings.Contains(headerPart, "X-TUN-MITM-Test:") {
						modifiedRsp := headerPart +
							fmt.Sprintf("\r\nX-TUN-MITM-Test: %s", testNonce) +
							rspStr[headerEndPos:]
						log.Infof("[MITM] Added test nonce to response for yaklang.com")
						log.Infof("[MITM] Modified Response Headers (with nonce):")
						// 重新解析修改后的响应以打印
						modifiedRspParsed, err := utils.ReadHTTPResponseFromBytes([]byte(modifiedRsp), req)
						if err == nil {
							log.Infof("[MITM]   Status: %s", modifiedRspParsed.Status)
							for key, values := range modifiedRspParsed.Header {
								for _, value := range values {
									log.Infof("[MITM]   %s: %s", key, value)
								}
							}
						}
						select {
						case testPassed <- true:
						default:
						}
						return []byte(modifiedRsp)
					} else {
						log.Infof("[MITM] Test nonce already present in response")
					}
				}
			}

			return rsp
		}),
	)
	if err != nil {
		return utils.Errorf("failed to create MITM server: %v", err)
	}

	// 10. 启动 MITM 服务器（监听本地端口，但实际上会从 channel 接收连接）
	mitmPort := 0 // 使用 0 表示不监听端口，只从 channel 接收连接
	mitmListener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", mitmPort))
	if err != nil {
		return utils.Errorf("failed to create MITM listener: %v", err)
	}
	actualPort := mitmListener.Addr().(*net.TCPAddr).Port
	log.Infof("MITM server listening on port: %d", actualPort)

	// 启动 MITM 服务器
	go func() {
		defer mitmListener.Close()
		err := mitmServer.ServerListener(ctx, mitmListener)
		if err != nil && ctx.Err() == nil {
			log.Errorf("MITM server error: %v", err)
		}
	}()

	// 等待 MITM 服务器启动
	time.Sleep(1 * time.Second)

	// 11. 启动测试：访问 https://yaklang.com
	go func() {
		time.Sleep(2 * time.Second) // 等待路由和 MITM 服务器生效

		log.Infof("===========================================")
		log.Infof("║ Starting Test: https://yaklang.com      ║")
		log.Infof("===========================================")

		// 使用 HTTP 客户端访问 https://yaklang.com
		// 由于路由表劫持，这个请求会被发送到 TUN 设备
		client := &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					// 直接连接，会被路由表劫持到 TUN
					return net.DialTimeout(network, addr, 10*time.Second)
				},
			},
		}

		req, err := http.NewRequestWithContext(ctx, "GET", "https://yaklang.com", nil)
		if err != nil {
			log.Errorf("[Test] failed to create request: %v", err)
			return
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Errorf("[Test] failed to send request: %v", err)
			return
		}
		defer resp.Body.Close()

		// 打印完整的 HTTP Response Header（包括状态行和所有 header）
		log.Infof("[Test] ==========================================")
		log.Infof("[Test] HTTP Response Status: %s", resp.Status)
		log.Infof("[Test] HTTP Response Headers:")
		for key, values := range resp.Header {
			for _, value := range values {
				log.Infof("[Test]   %s: %s", key, value)
			}
		}
		log.Infof("[Test] ==========================================")

		// 检查状态码，必须是 200 OK
		if resp.StatusCode != 200 {
			log.Errorf("[Test] ✗ FAILED - Response status is %s, expected 200 OK", resp.Status)
			return
		}

		// 检查响应头中是否有测试 nonce
		testHeader := resp.Header.Get("X-TUN-MITM-Test")
		if testHeader == testNonce {
			log.Infof("[Test] ✓ PASSED - Test nonce found in response header: %s", testHeader)
			select {
			case testPassed <- true:
			default:
			}
		} else {
			log.Errorf("[Test] ✗ FAILED - Test nonce not found in response header (got: %s, expected: %s)", testHeader, testNonce)
		}

		// 读取响应体的一部分用于验证
		buf := make([]byte, 1024)
		n, _ := resp.Body.Read(buf)
		if n > 0 {
			log.Infof("[Test] received %d bytes of response body", n)
		}
	}()

	// 12. 监听设备事件
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

	// 13. 等待测试完成
	log.Infof("waiting for test to complete...")
	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-testPassed:
			log.Infof("===========================================")
			log.Infof("║ ✓ TEST PASSED - SHUTTING DOWN...       ║")
			log.Infof("===========================================")
			hijackedURLsMu.Lock()
			log.Infof("Hijacked URLs: %v", hijackedURLs)
			hijackedURLsMu.Unlock()
			time.Sleep(500 * time.Millisecond)
			return nil
		case <-timeout:
			log.Errorf("===========================================")
			log.Errorf("║ ✗ TEST TIMEOUT AFTER 30s              ║")
			log.Errorf("===========================================")
			hijackedURLsMu.Lock()
			log.Infof("Hijacked URLs: %v", hijackedURLs)
			hijackedURLsMu.Unlock()
			return utils.Errorf("test timeout")
		case <-ctx.Done():
			log.Infof("test interrupted by user")
			return nil
		}
	}
}

// GetPublicNetworkInterface 获取公网网卡和IP地址
func GetPublicNetworkInterface() (interfaceName string, ipAddr string, err error) {
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

// resetForwardTunPassword 重置 forward-tun-to-socks 高权限进程的密码
func resetForwardTunPassword(c *cli.Context) error {
	fmt.Println("[*] Resetting forward-tun-to-socks password...")

	newSecret, err := lowtun.ResetPrivilegedSecret()
	if err != nil {
		fmt.Printf("[!] Failed to reset password: %v\n", err)
		return err
	}

	fmt.Printf("[+] Successfully reset password to: %s\n", newSecret)
	fmt.Println("[*] The old privileged process will no longer be able to authenticate.")
	fmt.Println("[*] Next time you try to use TUN, it will trigger the privileged kill process.")

	return nil
}
