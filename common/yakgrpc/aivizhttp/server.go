package aivizhttp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// VizHTTPServer 是 AI Agent 可视化监控的 HTTP 服务.
// 与 gRPC server 同进程运行, 通过 aireact 进程内注册表订阅实时事件,
// 从 SQLite profile DB 读取历史事件进行回放分析.
// 关键词: viz server, agent monitor, dashboard
type VizHTTPServer struct {
	config *VizServerConfig
	db     *gorm.DB

	router     *mux.Router
	httpServer *http.Server

	// actualPort 是实际监听的端口 (可能与 config.Port 不同, 当默认端口被占用时自动分配)
	actualPort int

	ctx    context.Context
	cancel context.CancelFunc
}

// NewVizHTTPServer 创建可视化监控服务
func NewVizHTTPServer(opts ...Option) (*VizHTTPServer, error) {
	config := NewDefaultConfig()
	for _, opt := range opts {
		opt(config)
	}
	config.fillDefaults()

	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Error("project database is unavailable")
	}

	// 确保 AI 相关表已建好 (AISession, AiOutputEvent 等在 Project DB 下)
	schema.AutoMigrate(db, schema.KEY_SCHEMA_YAKIT_DATABASE)
	schema.ApplyPatches(db, schema.KEY_SCHEMA_YAKIT_DATABASE)

	ctx, cancel := context.WithCancel(context.Background())
	s := &VizHTTPServer{
		config: config,
		db:     db,
		ctx:    ctx,
		cancel: cancel,
	}

	s.registerRoutes()
	return s, nil
}

// Start 启动 HTTP 服务 (阻塞).
// 端口选择策略: 如果配置端口为 0, 或被占用, 直接使用 utils.GetRandomAvailableTCPPort()
// 获取一个系统分配的随机可用端口, 避免固定回退导致用户看到的 startup banner URL
// 与实际端口不一致.
func (s *VizHTTPServer) Start() error {
	port := s.config.Port
	if port <= 0 || !utils.IsPortAvailable(s.config.Host, port) {
		if port > 0 {
			log.Warnf("port %d is in use, using random available port", port)
		}
		port = utils.GetRandomAvailableTCPPort()
	}
	s.actualPort = port

	addr := utils.HostPort(s.config.Host, port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  0,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	if s.config.AuthToken != "" {
		log.Infof("bearer authentication enabled")
	} else {
		log.Infof("authentication disabled (no auth_token configured)")
	}

	printVizServerStartupInfo(s)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return utils.Errorf("http server error: %v", err)
	}
	return nil
}

func printVizServerStartupInfo(s *VizHTTPServer) {
	addr := s.GetAddr()
	prefix := s.GetRoutePrefix()
	fmt.Println("============================================")
	fmt.Println("         Yaklang Agent Viz Server")
	fmt.Println("============================================")
	fmt.Printf("  Listening:    http://%s\n", addr)
	fmt.Printf("  Prefix:       %s\n", prefix)
	fmt.Printf("  Health:       http://%s%s/health\n", addr, prefix)
	fmt.Printf("  Sessions:     http://%s%s/sessions\n", addr, prefix)
	fmt.Printf("  Live Agents:  http://%s%s/live\n", addr, prefix)
	fmt.Printf("  Events SSE:   http://%s%s/sessions/{id}/stream\n", addr, prefix)
	if s.IsFrontendEnabled() {
		fmt.Printf("  Dashboard:    http://%s/   (built-in monitor UI)\n", addr)
	}
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop.")
	fmt.Println()
}

// ServeHTTP 暴露 router, 便于测试使用 httptest
func (s *VizHTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.router == nil {
		http.NotFound(w, r)
		return
	}
	s.router.ServeHTTP(w, r)
}

// Shutdown 优雅关闭
func (s *VizHTTPServer) Shutdown() {
	log.Info("shutting down viz http server...")
	s.cancel()
	if s.httpServer != nil {
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		_ = s.httpServer.Shutdown(shutCtx)
	}
	log.Info("viz http server stopped")
}

// GetAddr 返回实际监听地址 (可能因端口冲突而与配置不同)
func (s *VizHTTPServer) GetAddr() string {
	port := s.actualPort
	if port == 0 {
		port = s.config.Port
	}
	return utils.HostPort(s.config.Host, port)
}

// GetRoutePrefix 返回路由前缀
func (s *VizHTTPServer) GetRoutePrefix() string {
	return s.config.RoutePrefix
}

// IsFrontendEnabled 返回是否启用了内置前端页面
func (s *VizHTTPServer) IsFrontendEnabled() bool {
	return s.config.ServeFrontend
}
