package yakcmds

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func routeManagerToSocks(c *cli.Context) error {
	socketPath := c.String("socket-path")
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

	log.Infof("ready to accept client connections on socket: %s", socketPath)

	type clientInfo struct {
		id     int
		conn   net.Conn
		writer *protocolWriter
		reader *protocolReader
		cancel context.CancelFunc
	}

	var (
		clientsMu sync.RWMutex
		clients   = make(map[int]*clientInfo)
		nextID    = 1
	)

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
				if err := authenticateRouteConnection(conn, secret); err != nil {
					log.Errorf("authentication failed: %v", err)
					conn.Close()
					continue
				}
				log.Infof("authentication successful")
			} else {
				// 未认证模式也需要发送初始响应
				if err := authenticateRouteConnection(conn, ""); err != nil {
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
				reader: &protocolReader{conn: conn, mtu: 1400},
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

				for {
					modifyRequest, err := readRouteModifyRequest(cli.reader)
					if err != nil {
						log.Errorf("failed to read route modify request: %v", err)
					}

					var successList = make([]string, 0)
					var failMap = make(map[string]error)
					var gErr error

					switch true {
					case modifyRequest.IsAdd():
						successList, failMap = netutil.BatchAddSpecificIPRouteToNetInterface(modifyRequest.IpList, modifyRequest.TunName)
					case modifyRequest.IsDelete():
						if len(modifyRequest.IpList) > 0 {
							successList, failMap = netutil.BatchDeleteSpecificIPRoute(modifyRequest.IpList)
						} else if modifyRequest.TunName != "" {
							successList, failMap, gErr = netutil.DeleteAllRoutesForInterface(modifyRequest.TunName)
						}
					default:
						log.Errorf("unknown route modify request: %v", modifyRequest)
					}

					err = writeRouteModifyResponse(client.writer, successList, failMap, gErr)
					if err != nil {
						log.Errorf("failed to write route modify response: %v", err)
					}
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

// authenticateRouteConnection 服务器端认证：读取 {"secret": "..."} 并验证，然后回复 {"ok": true} 或 {"ok": false, "error": "..."}
func authenticateRouteConnection(conn net.Conn, expectedSecret string) error {
	// 1. 读取认证请求
	reqData, err := readRouteData(conn)
	if err != nil {
		return utils.Errorf("failed to read auth request data: %v", err)
	}

	log.Debugf("received auth request: %s", string(reqData))

	// 2. 解析认证请求
	var authReq map[string]string
	if err := json.Unmarshal(reqData, &authReq); err != nil {
		sendRouteAuthResponse(conn, false, "invalid auth request format")
		return utils.Errorf("failed to unmarshal auth request: %v", err)
	}

	// 3. 验证密码
	clientSecret, exists := authReq["secret"]
	if !exists {
		sendRouteAuthResponse(conn, false, "missing secret field")
		return utils.Errorf("missing secret field in auth request")
	}

	if clientSecret != expectedSecret {
		sendRouteAuthResponse(conn, false, "invalid secret")
		return utils.Errorf("invalid secret: expected %s, got %s", expectedSecret, clientSecret)
	}

	// 4. 认证成功，发送响应
	if err := sendRouteAuthResponse(conn, true, ""); err != nil {
		return utils.Errorf("failed to send auth response: %v", err)
	}

	log.Debugf("authentication successful")
	return nil
}

// sendRouteAuthResponse 发送认证响应，包含 utun 名称
func sendRouteAuthResponse(conn net.Conn, ok bool, errMsg string) error {
	resp := map[string]interface{}{
		"ok": ok,
	}
	if errMsg != "" {
		resp["error"] = errMsg
	}

	respData, err := json.Marshal(resp)
	if err != nil {
		return utils.Errorf("failed to marshal auth response: %v", err)
	}

	err = sendRouteData(conn, respData)
	if err != nil {
		return utils.Errorf("failed to send auth response: %v", err)
	}

	log.Debugf("sent auth response: %s", string(respData))
	return nil
}

func sendRouteData(conn net.Conn, data []byte) error {
	var lengthBuf [4]byte
	binary.BigEndian.PutUint32(lengthBuf[:], uint32(len(data)))
	if _, err := conn.Write(lengthBuf[:]); err != nil {
		return utils.Errorf("failed to write auth response length: %v", err)
	}
	if _, err := conn.Write(data); err != nil {
		return utils.Errorf("failed to write data: %v", err)
	}
	return nil
}

func readRouteData(conn net.Conn) ([]byte, error) {
	var lengthBuf [4]byte
	if _, err := io.ReadFull(conn, lengthBuf[:]); err != nil {
		return nil, utils.Errorf("failed to prefix length: %v", err)
	}

	prefix := int(binary.BigEndian.Uint32(lengthBuf[:]))
	if prefix <= 0 || prefix > 1024 {
		return nil, utils.Errorf("invalid perfix length: %d", prefix)
	}

	data := make([]byte, prefix)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, utils.Errorf("failed to read data: %v", err)
	}

	return data, nil
}

func readRouteModifyRequest(reader *protocolReader) (*netutil.RouteModifyMessage, error) {
	var rawData = make([]byte, 1500)
	_, err := reader.Read(rawData)
	if err != nil {
		return nil, err
	}
	var message netutil.RouteModifyMessage
	err = json.Unmarshal(rawData, &message)
	if err != nil {
		return nil, err
	}
	return &message, nil
}

func writeRouteModifyResponse(writer *protocolWriter, successList []string, failMap map[string]error, generalErr error) error {
	// 构造响应数据
	response := netutil.RouteModifyResult{
		SuccessList: successList,
		FailMap:     make(map[string]string),
	}

	failMapIns := response.FailMap

	for ip, err := range failMap {
		failMapIns[ip] = err.Error()
	}

	if generalErr != nil {
		response.Error = generalErr.Error()
	}
	// 序列化并发送响应
	respData, err := json.Marshal(response)
	if err != nil {
		return utils.Errorf("failed to marshal response data: %v", err)
	}
	_, err = writer.Write(respData)
	return err
}
