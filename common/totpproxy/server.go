package totpproxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// Server 反向代理服务器
type Server struct {
	config    *ServerConfig
	listener  net.Listener
	ctx       context.Context
	cancel    context.CancelFunc
	running   bool
	runningMu sync.RWMutex
}

// NewServer 创建反向代理服务器
func NewServer(opts ...ServerOption) *Server {
	config := NewDefaultServerConfig()
	for _, opt := range opts {
		opt(config)
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	// 验证核心配置是否就绪
	if err := s.config.Validate(); err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	var err error

	if s.config.EnableTLS {
		var tlsConfig *tls.Config
		tlsConfig, err = s.setupTLS()
		if err != nil {
			return fmt.Errorf("failed to setup TLS: %w", err)
		}
		s.listener, err = tls.Listen("tcp", s.config.ListenAddr, tlsConfig)
	} else {
		s.listener, err = net.Listen("tcp", s.config.ListenAddr)
	}

	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.config.ListenAddr, err)
	}

	s.setRunning(true)
	log.Infof("[totpproxy] server started on %s (TLS: %t)", s.config.ListenAddr, s.config.EnableTLS)
	log.Infof("[totpproxy] forwarding to %s (TLS: %t)", s.config.TargetAddr, s.config.TargetTLS)

	go s.acceptConnections()
	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	s.cancel()
	s.setRunning(false)
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// IsRunning 检查服务器是否运行中
func (s *Server) IsRunning() bool {
	s.runningMu.RLock()
	defer s.runningMu.RUnlock()
	return s.running
}

func (s *Server) setRunning(running bool) {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	s.running = running
}

// acceptConnections 接受连接
func (s *Server) acceptConnections() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				log.Infof("[totpproxy] server context done, closing listener")
				return
			default:
				if !strings.Contains(err.Error(), "use of closed network connection") {
					log.Errorf("[totpproxy] failed to accept connection: %v", err)
				}
				continue
			}
		}
		go s.handleConnection(conn)
	}
}

// handleConnection 处理单个连接
func (s *Server) handleConnection(conn net.Conn) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("[totpproxy] panic in handleConnection: %v", err)
		}
		conn.Close()
	}()

	reader := bufio.NewReader(conn)

	// 支持 HTTP/1.1 Keep-Alive
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			conn.SetReadDeadline(time.Now().Add(30 * time.Second))

			req, err := utils.ReadHTTPRequestFromBufioReader(reader)
			if err != nil {
				if err != io.EOF && !strings.Contains(err.Error(), "use of closed network connection") {
					if s.config.Debug {
						log.Debugf("[totpproxy] failed to read request: %v", err)
					}
				}
				return
			}

			reqRaw, err := utils.DumpHTTPRequest(req, true)
			if err != nil {
				log.Errorf("[totpproxy] failed to dump request: %v", err)
				s.writeErrorResponse(conn, 400, "Bad Request")
				return
			}

			// serveRequest 返回 false 表示应该关闭连接
			if !s.serveRequest(conn, reqRaw) {
				return
			}
		}
	}
}

// serveRequest 处理单个请求
// 返回 true 表示可以继续处理下一个请求（Keep-Alive），false 表示应该关闭连接
func (s *Server) serveRequest(clientConn net.Conn, reqRaw []byte) bool {
	reqPath := lowhttp.GetHTTPRequestPath(reqRaw)
	reqMethod := lowhttp.GetHTTPRequestMethod(reqRaw)

	if s.config.Debug {
		log.Infof("[totpproxy] %s %s from %s", reqMethod, reqPath, clientConn.RemoteAddr())
	}

	// 1. 路径白名单检查
	if !isPathAllowed(s.config, reqPath) {
		if s.config.Debug {
			log.Warnf("[totpproxy] path not allowed: %s", reqPath)
		}
		s.writeErrorResponse(clientConn, 404, "Not Found")
		return false // 关闭连接
	}

	// 2. TOTP 验证
	if err := verifyTOTP(s.config, reqRaw); err != nil {
		if s.config.Debug {
			log.Warnf("[totpproxy] TOTP verification failed from %s: %v", clientConn.RemoteAddr(), err)
		}
		s.writeErrorResponse(clientConn, 401, "Unauthorized: "+err.Error())
		return false // 关闭连接
	}

	// 3. 转发请求（流式转发后不支持 Keep-Alive，需要关闭连接）
	s.forwardRequest(clientConn, reqRaw)
	return false // 流式转发后关闭连接，不支持 Keep-Alive
}

// forwardRequest 使用双向流式转发请求到后端
func (s *Server) forwardRequest(clientConn net.Conn, reqRaw []byte) {
	// 准备请求：移除 TOTP 验证头、替换 Host 头
	reqRaw = s.prepareRequest(reqRaw)

	// 建立到后端的连接
	backendConn, err := s.dialBackend()
	if err != nil {
		log.Errorf("[totpproxy] failed to connect to backend: %v", err)
		s.writeErrorResponse(clientConn, 502, "Bad Gateway: "+err.Error())
		return
	}
	defer backendConn.Close()

	if s.config.Debug {
		log.Infof("[totpproxy] connected to backend %s", s.config.TargetAddr)
	}

	reqRaw = lowhttp.ReplaceHTTPPacketHeader(reqRaw, "Connection", "close")

	// 发送请求到后端
	_, err = backendConn.Write(reqRaw)
	if err != nil {
		log.Errorf("[totpproxy] failed to write request to backend: %v", err)
		s.writeErrorResponse(clientConn, 502, "Bad Gateway: "+err.Error())
		return
	}

	// 流式转发响应：后端 -> 客户端
	n, err := s.streamResponse(backendConn, clientConn)
	if err != nil && s.config.Debug {
		log.Debugf("[totpproxy] stream ended: %v", err)
	}

	if s.config.Debug {
		log.Infof("[totpproxy] response streamed, %d bytes total", n)
	}
}

// prepareRequest 准备转发的请求
func (s *Server) prepareRequest(reqRaw []byte) []byte {
	// 移除 TOTP 验证头
	reqRaw = lowhttp.DeleteHTTPPacketHeader(reqRaw, s.config.TOTPHeader)

	// 替换 Host 头
	targetHost, targetPort, _ := utils.ParseStringToHostPort(s.config.TargetAddr)
	var hostHeader string
	if (s.config.TargetTLS && targetPort == 443) || (!s.config.TargetTLS && targetPort == 80) {
		hostHeader = targetHost
	} else {
		hostHeader = utils.HostPort(targetHost, targetPort)
	}
	reqRaw = lowhttp.ReplaceHTTPPacketHeader(reqRaw, "Host", hostHeader)

	return reqRaw
}

// dialBackend 建立到后端的连接
func (s *Server) dialBackend() (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout: 30 * time.Second,
	}

	if s.config.TargetTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true, // 内部服务通常使用自签名证书
		}
		return tls.DialWithDialer(dialer, "tcp", s.config.TargetAddr, tlsConfig)
	}

	return dialer.Dial("tcp", s.config.TargetAddr)
}

// streamResponse 流式转发响应数据
// 返回传输的总字节数和可能的错误
func (s *Server) streamResponse(src, dst net.Conn) (int64, error) {
	buf := make([]byte, 32*1024) // 32KB 缓冲区
	var totalBytes int64

	for {
		// 设置读超时，每次读取操作重置
		src.SetReadDeadline(time.Now().Add(s.config.Timeout))

		n, readErr := src.Read(buf)
		if n > 0 {
			// 立即写入客户端，实现流式转发
			written, writeErr := dst.Write(buf[:n])
			totalBytes += int64(written)

			if writeErr != nil {
				return totalBytes, fmt.Errorf("write to client failed: %w", writeErr)
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				return totalBytes, nil // 正常结束
			}
			// 检查是否是超时错误
			if netErr, ok := readErr.(net.Error); ok && netErr.Timeout() {
				return totalBytes, fmt.Errorf("read timeout: %w", readErr)
			}
			return totalBytes, fmt.Errorf("read from backend failed: %w", readErr)
		}
	}
}

// writeErrorResponse 写入错误响应
func (s *Server) writeErrorResponse(conn net.Conn, statusCode int, message string) {
	statusText := map[int]string{
		400: "Bad Request",
		401: "Unauthorized",
		404: "Not Found",
		500: "Internal Server Error",
		502: "Bad Gateway",
	}[statusCode]

	if statusText == "" {
		statusText = "Error"
	}

	response := fmt.Sprintf("HTTP/1.1 %d %s\r\n"+
		"Content-Type: text/plain; charset=utf-8\r\n"+
		"Connection: close\r\n"+
		"Content-Length: %d\r\n"+
		"\r\n%s", statusCode, statusText, len(message), message)

	conn.Write([]byte(response))
}

// setupTLS 设置 TLS 配置
func (s *Server) setupTLS() (*tls.Config, error) {
	if len(s.config.TLSCert) == 0 || len(s.config.TLSKey) == 0 {
		return nil, ErrMissingTLSConfig
	}

	cert, err := tls.X509KeyPair(s.config.TLSCert, s.config.TLSKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS key pair: %w", err)
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
}
