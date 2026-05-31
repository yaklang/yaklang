package airaghttp

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// 集合名清理正则: 仅保留字母数字下划线连字符
var collectionNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// RAGHTTPServer 纯 HTTP 的 RAG 知识库服务 (无 gRPC 依赖)
// 关键词: airaghttp server, standalone http, no grpc, no cors
type RAGHTTPServer struct {
	config *RAGServerConfig
	db     *gorm.DB

	// embeddingClient 可选的嵌入客户端覆盖. 为 nil 时使用默认嵌入服务.
	// 主要用于测试 (mock embedder) 与高级自定义部署.
	embeddingClient aispec.EmbeddingCaller

	// readyCollections 启动时确定的可用知识库集合 (启动后只读)
	readyCollections []string

	// 并发信号量 (yak 无 select, 用 Mutex+int 实现非阻塞抢占)
	inflightLock  sync.Mutex
	inflightCount int

	router     *mux.Router
	httpServer *http.Server

	ctx    context.Context
	cancel context.CancelFunc
}

// NewRAGHTTPServer 创建并完成启动期校验
// 关键词: New server, startup validation, no available knowledge base -> error
// 若最终没有任何可用知识库, 返回 error, 命令应据此拒绝启动.
func NewRAGHTTPServer(config *RAGServerConfig, opts ...Option) (*RAGHTTPServer, error) {
	return newRAGHTTPServerWithDeps(config, nil, nil, opts...)
}

// newRAGHTTPServerWithDeps 允许注入自定义 db 与 embedding 客户端 (测试 / 高级用法)
// db 为 nil 时使用全局 profile DB; embeddingClient 为 nil 时使用默认嵌入服务.
func newRAGHTTPServerWithDeps(config *RAGServerConfig, db *gorm.DB, embeddingClient aispec.EmbeddingCaller, opts ...Option) (*RAGHTTPServer, error) {
	if config == nil {
		config = NewDefaultConfig()
	}
	for _, opt := range opts {
		opt(config)
	}
	config.fillDefaults()

	if db == nil {
		db = consts.GetGormProfileDatabase()
	}
	if db == nil {
		return nil, utils.Error("profile database is unavailable")
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &RAGHTTPServer{
		config:          config,
		db:              db,
		embeddingClient: embeddingClient,
		ctx:             ctx,
		cancel:          cancel,
	}

	if err := s.resolveReadyCollections(); err != nil {
		cancel()
		return nil, err
	}

	// 启动检测: 高质/轻量两个通道分别是否配置好(api_key); 未配置则回退内置轻量模型.
	// 关键词: startup ai check, quality/speed channel, fallback lightweight
	if config.IsAIConfigured() {
		log.Infof("quality channel: using custom high-quality model [%s:%s]", config.AI.Type, modelOrDefault(config.AI.Model))
	} else {
		log.Warnf("quality channel: ai.api_key empty, falling back to lightweight model [%s]", LightweightModelName)
	}
	if config.IsLightweightAIConfigured() {
		log.Infof("speed channel: using custom lightweight model [%s:%s]", config.AILightweight.Type, modelOrDefault(config.AILightweight.Model))
	} else {
		log.Infof("speed channel: using built-in lightweight model [%s]", LightweightModelName)
	}

	s.registerRoutes()
	return s, nil
}

// resolveReadyCollections 解析启动时可用的知识库集合
// 顺序: 先导入本地 rag_files, 再合并 config.Collections 或 profile DB 内全部集合
func (s *RAGHTTPServer) resolveReadyCollections() error {
	ready := make([]string, 0)
	seen := make(map[string]bool)

	for _, f := range s.config.RagFiles {
		name, err := s.importRagFile(f)
		if err != nil {
			return err
		}
		if name != "" && !seen[name] {
			ready = append(ready, name)
			seen[name] = true
		}
	}

	var candidates []string
	if len(s.config.Collections) > 0 {
		candidates = s.config.Collections
	} else {
		candidates = rag.ListCollections(s.db)
	}

	for _, name := range candidates {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		if !rag.CollectionIsExists(s.db, name) {
			log.Warnf("configured collection not found, skip: %s", name)
			continue
		}
		ready = append(ready, name)
		seen[name] = true
	}

	if len(ready) == 0 {
		return utils.Error("no available knowledge base; please run `yak rag-download` or provide local .rag files first")
	}

	s.readyCollections = ready
	log.Infof("rag http server ready with %d knowledge base(s): %v", len(ready), ready)
	return nil
}

// importRagFile 导入单个本地 .rag 文件, 返回确定的集合名
// 关键词: rag.ImportRAG, deterministic collection name, skip if exists
func (s *RAGHTTPServer) importRagFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if !utils.IsFile(path) {
		return "", utils.Errorf("rag file not found: %s", path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil || absPath == "" {
		absPath = path
	}

	baseName := filepath.Base(absPath)
	baseName = strings.TrimSuffix(baseName, ".rag")
	safeName := collectionNameSanitizer.ReplaceAllString(baseName, "_")
	if safeName == "" {
		safeName = "kb"
	}
	pathHash := utils.CalcSha256(absPath)
	kbName := fmt.Sprintf("rag_%s_%s", pathHash[:8], safeName)

	if rag.CollectionIsExists(s.db, kbName) {
		log.Infof("collection already exists, skip importing: %s", kbName)
		return kbName, nil
	}

	log.Infof("importing rag file %s -> collection %s", absPath, kbName)
	importErr := rag.ImportRAG(absPath,
		rag.WithDB(s.db),
		rag.WithRAGCtx(s.ctx),
		rag.WithName(kbName),
		rag.WithExportOverwriteExisting(false),
		rag.WithProgressHandler(func(percent float64, message string, messageType string) {
			if percent == 0 || percent >= 100 {
				log.Infof("[import %.0f%%] %s", percent, message)
			}
		}),
	)
	if importErr != nil {
		return "", utils.Errorf("import rag file %s failed: %v", absPath, importErr)
	}
	log.Infof("rag file imported successfully: %s", kbName)
	return kbName, nil
}

// registerRoutes 注册路由 (统一前缀, 放开跨域, 可选认证)
func (s *RAGHTTPServer) registerRoutes() {
	s.router = mux.NewRouter()
	sub := s.router.PathPrefix(s.config.RoutePrefix).Subrouter()

	sub.Use(corsMiddleware)
	if s.config.AuthToken != "" {
		sub.Use(s.authMiddleware)
	}

	sub.HandleFunc("/health", s.handleHealth).Methods(http.MethodGet, http.MethodOptions)
	sub.HandleFunc("/collections", s.handleCollections).Methods(http.MethodGet, http.MethodOptions)
	sub.HandleFunc("/search", s.handleSearch).Methods(http.MethodPost, http.MethodOptions)
	sub.HandleFunc("/chat", s.handleChat).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)

	// 内置只读前端页面 (不走鉴权, 便于浏览器直接打开; API 调用仍按配置鉴权)
	if s.config.ServeFrontend {
		s.router.HandleFunc("/", s.handleFrontend).Methods(http.MethodGet)
		s.router.HandleFunc("/index.html", s.handleFrontend).Methods(http.MethodGet)
	}
}

// handleFrontend 返回内置的只读搜索页面 (路由前缀注入)
func (s *RAGHTTPServer) handleFrontend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(renderFrontendHTML(s.config.RoutePrefix)))
}

// Start 启动 HTTP 服务 (阻塞)
func (s *RAGHTTPServer) Start() error {
	addr := utils.HostPort(s.config.Host, s.config.Port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  0,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	log.Infof("rag http server starting on %s (prefix: %s)", addr, s.config.RoutePrefix)
	if s.config.AuthToken != "" {
		log.Infof("bearer authentication enabled")
	} else {
		log.Infof("authentication disabled (no auth_token configured)")
	}

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return utils.Errorf("http server error: %v", err)
	}
	return nil
}

// ServeHTTP 暴露 router, 便于测试使用 httptest
func (s *RAGHTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.router == nil {
		http.NotFound(w, r)
		return
	}
	s.router.ServeHTTP(w, r)
}

// Shutdown 优雅关闭
func (s *RAGHTTPServer) Shutdown() {
	log.Info("shutting down rag http server...")
	s.cancel()
	if s.httpServer != nil {
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		_ = s.httpServer.Shutdown(shutCtx)
	}
	log.Info("rag http server stopped")
}

// GetAddr 返回监听地址
func (s *RAGHTTPServer) GetAddr() string {
	return utils.HostPort(s.config.Host, s.config.Port)
}

// GetRoutePrefix 返回路由前缀
func (s *RAGHTTPServer) GetRoutePrefix() string {
	return s.config.RoutePrefix
}

// IsFrontendEnabled 返回是否启用了内置前端页面
func (s *RAGHTTPServer) IsFrontendEnabled() bool {
	return s.config.ServeFrontend
}

// GetReadyCollections 返回当前可用的知识库集合 (拷贝)
func (s *RAGHTTPServer) GetReadyCollections() []string {
	out := make([]string, len(s.readyCollections))
	copy(out, s.readyCollections)
	return out
}

// modelOrDefault 模型名为空时返回占位描述
func modelOrDefault(model string) string {
	if model == "" {
		return "(type default)"
	}
	return model
}

// GetAIModeDescription 返回当前 AI 模型的人类可读描述 (用于启动信息展示)
// 展示 质量优先 / 速度优先 两个通道当前所用模型.
func (s *RAGHTTPServer) GetAIModeDescription() string {
	quality := fmt.Sprintf("lightweight %s", LightweightModelName)
	if s.config.IsAIConfigured() {
		quality = fmt.Sprintf("custom %s:%s", s.config.AI.Type, modelOrDefault(s.config.AI.Model))
	}
	speed := fmt.Sprintf("built-in %s", LightweightModelName)
	if s.config.IsLightweightAIConfigured() {
		speed = fmt.Sprintf("custom %s:%s", s.config.AILightweight.Type, modelOrDefault(s.config.AILightweight.Model))
	}
	return fmt.Sprintf("quality=[%s] speed=[%s]", quality, speed)
}

// ========== 并发信号量 ==========

// acquireSlot 非阻塞抢占, 成功返回 true, 达到上限返回 false
func (s *RAGHTTPServer) acquireSlot() bool {
	s.inflightLock.Lock()
	defer s.inflightLock.Unlock()
	if s.inflightCount >= s.config.Concurrent {
		return false
	}
	s.inflightCount++
	return true
}

func (s *RAGHTTPServer) releaseSlot() {
	s.inflightLock.Lock()
	defer s.inflightLock.Unlock()
	if s.inflightCount > 0 {
		s.inflightCount--
	}
}

func (s *RAGHTTPServer) getInflight() int {
	s.inflightLock.Lock()
	defer s.inflightLock.Unlock()
	return s.inflightCount
}
