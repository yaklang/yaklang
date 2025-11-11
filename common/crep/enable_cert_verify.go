package crep

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// VerifyMITMRootCertInstalled 验证 MITM 根证书是否已正确安装到系统信任库
// 验证方法：
// 1. 使用 MITM 根证书启动一个临时 HTTPS 服务器
// 2. 使用系统证书池（不跳过验证）连接到该服务器
// 3. 如果连接成功且 TLS 握手完成，说明证书已正确安装
func VerifyMITMRootCertInstalled() error {
	log.Info("verifying MITM root certificate installation...")

	// 初始化 MITM 证书
	InitMITMCert()

	// 获取 CA 证书和私钥
	caCert, caKey, err := GetDefaultMITMCAAndPriv()
	if err != nil {
		return utils.Errorf("failed to get MITM CA: %v", err)
	}

	// 为测试生成一个服务器证书
	testDomain := "yaklang-verify.local"
	serverCert, err := FakeCertificateByHost(caCert, caKey, testDomain)
	if err != nil {
		return utils.Errorf("failed to generate test certificate: %v", err)
	}

	// 重要：移除证书链中的 CA 证书，只保留服务器证书
	// 这样客户端必须从系统证书池中查找 CA 才能验证
	serverCertWithoutCA := tls.Certificate{
		Certificate: [][]byte{serverCert.Certificate[0]}, // 只保留服务器证书，不包含 CA
		PrivateKey:  serverCert.PrivateKey,
		Leaf:        serverCert.Leaf,
	}

	// 启动临时 HTTPS 服务器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return utils.Errorf("failed to create listener: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()
	log.Infof("starting test HTTPS server on %s", serverAddr)

	// 创建 TLS 配置
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCertWithoutCA},
		MinVersion:   tls.VersionTLS12,
	}

	// 启动服务器
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		}),
		TLSConfig: tlsConfig,
	}

	serverErrCh := make(chan error, 1)
	go func() {
		tlsListener := tls.NewListener(listener, tlsConfig)
		if err := server.Serve(tlsListener); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
		}
		close(serverErrCh)
	}()

	// 等待服务器启动
	time.Sleep(200 * time.Millisecond)

	// 确保服务器最终会关闭
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	// 检查服务器是否正常启动
	select {
	case err := <-serverErrCh:
		if err != nil {
			return utils.Errorf("test server failed to start: %v", err)
		}
	default:
		// 服务器正常运行
	}

	// 客户端连接验证
	log.Info("attempting to connect with system certificate pool...")

	// 使用系统证书池（启用证书验证）
	systemCertPool, err := x509.SystemCertPool()
	if err != nil {
		return utils.Errorf("failed to load system cert pool: %v", err)
	}

	clientTLSConfig := &tls.Config{
		RootCAs:    systemCertPool,
		ServerName: testDomain,
		MinVersion: tls.VersionTLS12,
	}

	// 连接到测试服务器
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
	}

	conn, err := dialer.Dial("tcp", serverAddr)
	if err != nil {
		return utils.Errorf("failed to connect to test server: %v", err)
	}
	defer conn.Close()

	// 执行 TLS 握手
	tlsConn := tls.Client(conn, clientTLSConfig)
	tlsConn.SetDeadline(time.Now().Add(5 * time.Second))

	err = tlsConn.Handshake()
	if err != nil {
		return utils.Errorf("TLS handshake failed - certificate not trusted by system: %v", err)
	}

	// 验证连接状态
	state := tlsConn.ConnectionState()
	if !state.HandshakeComplete {
		return utils.Error("TLS handshake not complete")
	}

	if len(state.PeerCertificates) == 0 {
		return utils.Error("no peer certificates received")
	}

	// 验证证书链
	log.Infof("TLS handshake successful, certificate chain verified")
	log.Infof("peer certificate CN: %s", state.PeerCertificates[0].Subject.CommonName)

	// 发送一个简单的 HTTP 请求来确保连接正常
	_, err = tlsConn.Write([]byte("GET / HTTP/1.1\r\nHost: " + testDomain + "\r\n\r\n"))
	if err != nil {
		return utils.Errorf("failed to write to connection: %v", err)
	}

	// 读取响应
	buf := make([]byte, 1024)
	tlsConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := tlsConn.Read(buf)
	if err != nil {
		return utils.Errorf("failed to read response: %v", err)
	}

	response := string(buf[:n])
	if !utils.MatchAllOfSubString(response, "HTTP", "200") {
		return utils.Errorf("unexpected response: %s", response)
	}

	log.Info("✓ MITM root certificate is correctly installed and trusted by system")
	return nil
}

// VerifyMITMRootCertNotInstalled 验证 MITM 根证书是否已从系统信任库中移除
// 验证方法：与 VerifyMITMRootCertInstalled 相反，连接应该失败
func VerifyMITMRootCertNotInstalled() error {
	log.Info("verifying MITM root certificate is NOT installed...")

	err := VerifyMITMRootCertInstalled()
	if err == nil {
		return utils.Error("certificate is still installed and trusted by system")
	}

	if utils.MatchAllOfSubString(err.Error(), "not trusted", "certificate") ||
		utils.MatchAllOfSubString(err.Error(), "TLS handshake failed") {
		log.Info("✓ MITM root certificate is correctly removed from system trust")
		return nil
	}

	// 其他错误不算验证成功
	return utils.Errorf("verification failed with unexpected error: %v", err)
}

// QuickVerifyMITMRootCert 快速验证证书状态（不启动服务器，只检查系统证书池）
func QuickVerifyMITMRootCert() (bool, error) {
	InitMITMCert()

	ca, _, err := GetDefaultCaAndKey()
	if err != nil {
		return false, utils.Errorf("failed to get CA certificate: %v", err)
	}

	// 解析 CA 证书（从 PEM 格式）
	block, _ := pem.Decode(ca)
	if block == nil {
		return false, utils.Error("failed to decode CA certificate from PEM")
	}

	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, utils.Errorf("failed to parse CA certificate: %v", err)
	}

	// 获取系统证书池
	systemPool, err := x509.SystemCertPool()
	if err != nil {
		return false, utils.Errorf("failed to get system cert pool: %v", err)
	}

	// 创建一个临时证书池用于验证
	tempPool := x509.NewCertPool()
	tempPool.AddCert(caCert)

	// 尝试验证 CA 证书本身
	opts := x509.VerifyOptions{
		Roots:     systemPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	_, err = caCert.Verify(opts)
	if err == nil {
		return true, nil
	}

	return false, nil
}

// TestCertificateOperations 测试证书的安装、验证和撤销操作
func TestCertificateOperations() error {
	log.Info("=== Testing Certificate Operations ===")

	// 1. 先移除可能存在的旧证书
	log.Info("Step 1: Removing any existing certificates...")
	_ = WithdrawMITMRootCertFromSystem() // 忽略错误，可能本来就不存在

	// 2. 验证证书确实不在系统中
	log.Info("Step 2: Verifying certificate is not installed...")
	err := VerifyMITMRootCertNotInstalled()
	if err != nil {
		return utils.Errorf("step 2 failed: %v", err)
	}

	// 3. 安装证书
	log.Info("Step 3: Installing certificate...")
	err = AddMITMRootCertIntoSystem()
	if err != nil {
		return utils.Errorf("step 3 failed: %v", err)
	}

	// 4. 验证证书已安装
	log.Info("Step 4: Verifying certificate is installed...")
	err = VerifyMITMRootCertInstalled()
	if err != nil {
		return utils.Errorf("step 4 failed: %v", err)
	}

	// 5. 再次安装（测试幂等性）
	log.Info("Step 5: Installing certificate again (idempotency test)...")
	err = AddMITMRootCertIntoSystem()
	if err != nil {
		return utils.Errorf("step 5 failed: %v", err)
	}

	// 6. 验证仍然正常
	log.Info("Step 6: Verifying certificate still works...")
	err = VerifyMITMRootCertInstalled()
	if err != nil {
		return utils.Errorf("step 6 failed: %v", err)
	}

	// 7. 撤销证书
	log.Info("Step 7: Withdrawing certificate...")
	err = WithdrawMITMRootCertFromSystem()
	if err != nil {
		return utils.Errorf("step 7 failed: %v", err)
	}

	// 8. 验证证书已移除
	log.Info("Step 8: Verifying certificate is removed...")
	err = VerifyMITMRootCertNotInstalled()
	if err != nil {
		return utils.Errorf("step 8 failed: %v", err)
	}

	// 9. 再次撤销（测试幂等性）
	log.Info("Step 9: Withdrawing certificate again (idempotency test)...")
	err = WithdrawMITMRootCertFromSystem()
	if err != nil {
		return utils.Errorf("step 9 failed: %v", err)
	}

	// 10. 最终验证
	log.Info("Step 10: Final verification...")
	err = VerifyMITMRootCertNotInstalled()
	if err != nil {
		return utils.Errorf("step 10 failed: %v", err)
	}

	log.Info("=== All Tests Passed ===")
	return nil
}
