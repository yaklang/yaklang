package aihttp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

type AIAgentHTTPGateway struct {
	host        string
	port        int
	routePrefix string
	debug       bool
	uploadDir   string

	enableJWT  bool
	jwtSecret  string
	enableTOTP bool
	totpSecret string

	db *gorm.DB

	grpcServer *grpc.Server
	grpcConn   *grpc.ClientConn
	yakClient  ypb.YakClient
	grpcAddr   string

	runManager *RunManager

	httpServer *http.Server
	router     *mux.Router

	ctx    context.Context
	cancel context.CancelFunc
}

func NewAIAgentHTTPGateway(opts ...GatewayOption) (*AIAgentHTTPGateway, error) {
	ctx, cancel := context.WithCancel(context.Background())
	gw := &AIAgentHTTPGateway{
		host:        "0.0.0.0",
		port:        8089,
		routePrefix: "/agent",
		uploadDir:   filepath.Join(consts.GetDefaultYakitBaseDir(), "aihttp-uploads"),
		ctx:         ctx,
		cancel:      cancel,
	}

	for _, opt := range opts {
		opt(gw)
	}

	if gw.db == nil {
		gw.db = consts.GetGormProfileDatabase()
	}
	if gw.db == nil {
		cancel()
		return nil, fmt.Errorf("profile database is unavailable")
	}
	if err := gw.ensureUploadDir(); err != nil {
		cancel()
		return nil, fmt.Errorf("init upload dir failed: %w", err)
	}
	gw.runManager = NewRunManager(ctx)

	setting, err := gw.GetSettingFromDB()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("load setting from db failed: %w", err)
	}
	gw.applySettingToRuntime(setting)

	if err := gw.initGRPC(); err != nil {
		cancel()
		return nil, fmt.Errorf("init gRPC failed: %w", err)
	}

	gw.router = mux.NewRouter()
	gw.registerRoutes()

	return gw, nil
}

func (gw *AIAgentHTTPGateway) initGRPC() error {
	grpcPort := utils.GetRandomAvailableTCPPort()
	gw.grpcAddr = utils.HostPort("127.0.0.1", grpcPort)

	gw.grpcServer = grpc.NewServer(
		grpc.MaxRecvMsgSize(100*1024*1024),
		grpc.MaxSendMsgSize(100*1024*1024),
	)

	s, err := yakgrpc.NewYakGRPCServerForHTTPGateway()
	if err != nil {
		return fmt.Errorf("create yakgrpc server: %w", err)
	}

	ypb.RegisterYakServer(gw.grpcServer, s)

	lis, err := net.Listen("tcp", gw.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC: %w", err)
	}

	go func() {
		if err := gw.grpcServer.Serve(lis); err != nil {
			log.Errorf("gRPC server stopped: %v", err)
		}
	}()

	if err := utils.WaitConnect(gw.grpcAddr, 10); err != nil {
		return fmt.Errorf("wait gRPC connect: %w", err)
	}

	conn, err := grpc.Dial(
		gw.grpcAddr,
		grpc.WithInsecure(),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(100*1024*1024),
			grpc.MaxCallSendMsgSize(100*1024*1024),
		),
	)
	if err != nil {
		return fmt.Errorf("dial gRPC: %w", err)
	}

	gw.grpcConn = conn
	gw.yakClient = ypb.NewYakClient(conn)
	return nil
}

func (gw *AIAgentHTTPGateway) getDB() *gorm.DB {
	return gw.db
}

func (gw *AIAgentHTTPGateway) Start() error {
	addr := utils.HostPort(gw.host, gw.port)
	gw.httpServer = &http.Server{
		Addr:         addr,
		Handler:      gw.router,
		ReadTimeout:  0,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	log.Infof("AI Agent HTTP Gateway starting on %s (prefix: %s)", addr, gw.routePrefix)
	if gw.enableJWT {
		log.Infof("JWT authentication enabled")
	}
	if gw.enableTOTP {
		log.Infof("TOTP authentication enabled")
	}
	log.Infof("Upload directory: %s", gw.uploadDir)

	if err := gw.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server error: %w", err)
	}
	return nil
}

func (gw *AIAgentHTTPGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if gw.router == nil {
		http.NotFound(w, r)
		return
	}
	gw.router.ServeHTTP(w, r)
}

func (gw *AIAgentHTTPGateway) Shutdown() {
	log.Info("shutting down AI Agent HTTP Gateway...")

	gw.cancel()

	if gw.httpServer != nil {
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		gw.httpServer.Shutdown(shutCtx)
	}

	gw.runManager.CancelAll()

	if gw.grpcConn != nil {
		gw.grpcConn.Close()
	}
	if gw.grpcServer != nil {
		done := make(chan struct{})
		go func() {
			gw.grpcServer.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			go gw.grpcServer.Stop()
		}
	}

	log.Info("AI Agent HTTP Gateway stopped")
}

func (gw *AIAgentHTTPGateway) GetSetting() (aiAgentChatSettingPayload, error) {
	return gw.GetSettingFromDB()
}

func (gw *AIAgentHTTPGateway) UpdateSetting(s aiAgentChatSettingPayload) error {
	_, err := gw.SaveSettingToDB(s)
	return err
}

func (gw *AIAgentHTTPGateway) GetAddr() string {
	return utils.HostPort(gw.host, gw.port)
}

func (gw *AIAgentHTTPGateway) GetRoutePrefix() string {
	return gw.routePrefix
}

func (gw *AIAgentHTTPGateway) GetUploadDir() string {
	return gw.uploadDir
}

func (gw *AIAgentHTTPGateway) IsJWTEnabled() bool {
	return gw.enableJWT
}

func (gw *AIAgentHTTPGateway) GetJWTSecret() string {
	return gw.jwtSecret
}

func (gw *AIAgentHTTPGateway) IsTOTPEnabled() bool {
	return gw.enableTOTP
}

func (gw *AIAgentHTTPGateway) GetTOTPSecret() string {
	return gw.totpSecret
}

// 临时使用，由于tiered ai配置还没确定，暂时将配置应用到全局
func (gw *AIAgentHTTPGateway) applySettingToRuntime(s aiAgentChatSettingPayload) {
	if s.AIService == "" {
		return
	}
	aiCfg := &ypb.ThirdPartyApplicationConfig{
		Type: s.AIService,
		ExtraParams: []*ypb.KVPair{
			{Key: "model", Value: s.AIModelName},
		},
	}
	modelConfigs := consts.BuildAIModelConfigs([]*ypb.ThirdPartyApplicationConfig{aiCfg})
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:            true,
		RoutingPolicy:      consts.RoutingPolicy(s.ReviewPolicy),
		DisableFallback:    s.DisableToolUse,
		IntelligentConfigs: modelConfigs,
		LightweightConfigs: modelConfigs,
		VisionConfigs:      modelConfigs,
	})
	log.Infof("applied AI config from DB: service=%s model=%s policy=%s", s.AIService, s.AIModelName, s.ReviewPolicy)
}

func (gw *AIAgentHTTPGateway) ensureUploadDir() error {
	if gw.uploadDir == "" {
		gw.uploadDir = filepath.Join(consts.GetDefaultYakitBaseDir(), "aihttp-uploads")
	}
	gw.uploadDir = filepath.Clean(gw.uploadDir)
	return os.MkdirAll(gw.uploadDir, 0o755)
}
