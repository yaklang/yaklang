package aihttp

import (
	"context"
	"net"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

// AIAgentHTTPGateway is the main HTTP gateway for AI Agent API
// It wraps the gRPC StartAIReAct interface and provides RESTful HTTP API
type AIAgentHTTPGateway struct {
	routePrefix    string
	router         *mux.Router
	grpcServer     *yakgrpc.Server
	grpcClient     ypb.YakClient
	grpcConn       *grpc.ClientConn
	grpcListener   net.Listener
	grpcTransport  *grpc.Server
	runManager     *RunManager
	defaultSetting *ypb.AIStartParams
	settingMutex   sync.RWMutex

	// Authentication
	jwtSecret      string
	totpSecret     string
	authEnabled    bool
	authType       AuthType
	authMiddleware func(http.Handler) http.Handler
}

// AuthType defines the authentication type
type AuthType int

const (
	AuthTypeNone AuthType = iota
	AuthTypeJWT
	AuthTypeTOTP
)

// NewAIAgentHTTPGateway creates a new AI Agent HTTP Gateway
func NewAIAgentHTTPGateway(opts ...GatewayOption) (*AIAgentHTTPGateway, error) {
	gw := &AIAgentHTTPGateway{
		routePrefix: "/agent",
		router:      mux.NewRouter(),
		runManager:  NewRunManager(),
		defaultSetting: &ypb.AIStartParams{
			UseDefaultAIConfig: true,
		},
		authType:    AuthTypeNone,
		authEnabled: false,
	}

	// Apply options
	for _, opt := range opts {
		opt(gw)
	}

	// Create gRPC server if not provided
	if gw.grpcServer == nil {
		server, err := yakgrpc.NewServer(
			yakgrpc.WithInitFacadeServer(false),
		)
		if err != nil {
			return nil, err
		}
		gw.grpcServer = server
	}

	// Start local gRPC server and create client
	if err := gw.startLocalGRPCServer(); err != nil {
		return nil, err
	}

	// Setup authentication middleware
	gw.setupAuthMiddleware()

	// Register routes
	gw.registerRoutes()

	log.Infof("AI Agent HTTP Gateway initialized with prefix: %s", gw.routePrefix)

	return gw, nil
}

// startLocalGRPCServer starts a local gRPC server and creates a client connection
func (gw *AIAgentHTTPGateway) startLocalGRPCServer() error {
	// Get a random available port for local gRPC server
	port := utils.GetRandomAvailableTCPPort()
	addr := utils.HostPort("127.0.0.1", port)

	// Create gRPC transport
	gw.grpcTransport = grpc.NewServer(
		grpc.MaxRecvMsgSize(100*1024*1024),
		grpc.MaxSendMsgSize(100*1024*1024),
	)

	// Register the yakgrpc server
	ypb.RegisterYakServer(gw.grpcTransport, gw.grpcServer)

	// Start listener
	var err error
	gw.grpcListener, err = net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	// Start serving in background
	go func() {
		if err := gw.grpcTransport.Serve(gw.grpcListener); err != nil {
			log.Errorf("gRPC server error: %v", err)
		}
	}()

	// Wait for server to be ready
	if err := utils.WaitConnect(addr, 5); err != nil {
		return err
	}

	// Create gRPC client connection
	gw.grpcConn, err = grpc.Dial(addr,
		grpc.WithInsecure(),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(100*1024*1024),
			grpc.MaxCallSendMsgSize(100*1024*1024),
		),
	)
	if err != nil {
		return err
	}

	// Create the client
	gw.grpcClient = ypb.NewYakClient(gw.grpcConn)

	log.Infof("Local gRPC server started on %s", addr)
	return nil
}

// GetGRPCClient returns the gRPC client
func (gw *AIAgentHTTPGateway) GetGRPCClient() ypb.YakClient {
	return gw.grpcClient
}

// setupAuthMiddleware configures the authentication middleware based on auth type
func (gw *AIAgentHTTPGateway) setupAuthMiddleware() {
	switch gw.authType {
	case AuthTypeJWT:
		gw.authMiddleware = gw.jwtAuthMiddleware
		gw.authEnabled = true
		log.Info("JWT authentication enabled for AI Agent HTTP Gateway")
	case AuthTypeTOTP:
		gw.authMiddleware = gw.totpAuthMiddleware
		gw.authEnabled = true
		log.Info("TOTP authentication enabled for AI Agent HTTP Gateway")
	default:
		gw.authMiddleware = func(next http.Handler) http.Handler {
			return next
		}
		gw.authEnabled = false
	}
}

// GetHTTPRouteHandler returns the HTTP handler for the gateway
func (gw *AIAgentHTTPGateway) GetHTTPRouteHandler() http.Handler {
	return gw.router
}

// GetRouter returns the underlying mux router
func (gw *AIAgentHTTPGateway) GetRouter() *mux.Router {
	return gw.router
}

// GetGRPCServer returns the underlying gRPC server
func (gw *AIAgentHTTPGateway) GetGRPCServer() *yakgrpc.Server {
	return gw.grpcServer
}

// GetDefaultSetting returns the current default AI settings
func (gw *AIAgentHTTPGateway) GetDefaultSetting() *ypb.AIStartParams {
	gw.settingMutex.RLock()
	defer gw.settingMutex.RUnlock()
	return gw.defaultSetting
}

// SetDefaultSetting updates the default AI settings
func (gw *AIAgentHTTPGateway) SetDefaultSetting(setting *ypb.AIStartParams) {
	gw.settingMutex.Lock()
	defer gw.settingMutex.Unlock()
	gw.defaultSetting = setting
}

// Shutdown gracefully shuts down the gateway
func (gw *AIAgentHTTPGateway) Shutdown(ctx context.Context) error {
	log.Info("Shutting down AI Agent HTTP Gateway")

	// Cancel all running tasks
	gw.runManager.CancelAll()

	// Close gRPC client connection
	if gw.grpcConn != nil {
		gw.grpcConn.Close()
	}

	// Stop gRPC server
	if gw.grpcTransport != nil {
		gw.grpcTransport.GracefulStop()
	}

	// Close listener
	if gw.grpcListener != nil {
		gw.grpcListener.Close()
	}

	return nil
}

// IsAuthEnabled returns whether authentication is enabled
func (gw *AIAgentHTTPGateway) IsAuthEnabled() bool {
	return gw.authEnabled
}

// GetAuthType returns the authentication type
func (gw *AIAgentHTTPGateway) GetAuthType() AuthType {
	return gw.authType
}
