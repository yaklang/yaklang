package aihttp

import (
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// GatewayOption is a function that configures the AIAgentHTTPGateway
type GatewayOption func(*AIAgentHTTPGateway)

// WithRoutePrefix sets the route prefix for all API endpoints
// Default is "/agent"
func WithRoutePrefix(prefix string) GatewayOption {
	return func(gw *AIAgentHTTPGateway) {
		gw.routePrefix = prefix
	}
}

// WithInitSetting sets the initial default AI settings
func WithInitSetting(setting *ypb.AIStartParams) GatewayOption {
	return func(gw *AIAgentHTTPGateway) {
		if setting != nil {
			gw.defaultSetting = setting
		}
	}
}

// WithGRPCServer sets the gRPC server instance
// If not provided, a new server will be created
func WithGRPCServer(server *yakgrpc.Server) GatewayOption {
	return func(gw *AIAgentHTTPGateway) {
		gw.grpcServer = server
	}
}

// WithJWTAuth enables JWT authentication with the given secret
// The JWT token should be passed in the Authorization header as "Bearer <token>"
func WithJWTAuth(secret string) GatewayOption {
	return func(gw *AIAgentHTTPGateway) {
		gw.jwtSecret = secret
		gw.authType = AuthTypeJWT
		gw.authEnabled = true
	}
}

// WithTOTP enables TOTP (Time-based One-Time Password) authentication
// The TOTP code should be passed in the X-TOTP-Code header
func WithTOTP(secret string) GatewayOption {
	return func(gw *AIAgentHTTPGateway) {
		gw.totpSecret = secret
		gw.authType = AuthTypeTOTP
		gw.authEnabled = true
	}
}

// WithRunManager sets a custom run manager
func WithRunManager(rm *RunManager) GatewayOption {
	return func(gw *AIAgentHTTPGateway) {
		if rm != nil {
			gw.runManager = rm
		}
	}
}
