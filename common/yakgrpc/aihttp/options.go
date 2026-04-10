package aihttp

import "github.com/jinzhu/gorm"

type GatewayOption func(*AIAgentHTTPGateway)

func WithRoutePrefix(prefix string) GatewayOption {
	return func(g *AIAgentHTTPGateway) {
		if prefix != "" {
			g.routePrefix = prefix
		}
	}
}

func WithHost(host string) GatewayOption {
	return func(g *AIAgentHTTPGateway) {
		if host != "" {
			g.host = host
		}
	}
}

func WithPort(port int) GatewayOption {
	return func(g *AIAgentHTTPGateway) {
		if port > 0 {
			g.port = port
		}
	}
}

func WithJWTAuth(secret string) GatewayOption {
	return func(g *AIAgentHTTPGateway) {
		if secret != "" {
			g.jwtSecret = secret
			g.enableJWT = true
		}
	}
}

func WithTOTP(secret string) GatewayOption {
	return func(g *AIAgentHTTPGateway) {
		if secret != "" {
			g.totpSecret = secret
			g.enableTOTP = true
		}
	}
}

func WithDebug(debug bool) GatewayOption {
	return func(g *AIAgentHTTPGateway) {
		g.debug = debug
	}
}

func WithDatabase(db *gorm.DB) GatewayOption {
	return func(g *AIAgentHTTPGateway) {
		g.db = db
	}
}

func WithUploadDir(dir string) GatewayOption {
	return func(g *AIAgentHTTPGateway) {
		if dir != "" {
			g.uploadDir = dir
		}
	}
}
