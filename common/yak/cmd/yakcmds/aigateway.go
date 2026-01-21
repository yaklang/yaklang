package yakcmds

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/aihttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// AIHTTPGatewayCommand is the CLI command for starting AI HTTP Gateway
var AIHTTPGatewayCommand = &cli.Command{
	Name:    "ai-http-gateway",
	Aliases: []string{"ai-gateway", "aig"},
	Usage:   "Start AI Agent HTTP Gateway Server",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "host",
			Value: "0.0.0.0",
			Usage: "HTTP server host",
		},
		cli.IntFlag{
			Name:  "port",
			Value: 8089,
			Usage: "HTTP server port",
		},
		cli.StringFlag{
			Name:  "prefix",
			Value: "/agent",
			Usage: "API route prefix",
		},
		cli.StringFlag{
			Name:  "jwt-secret",
			Usage: "Enable JWT authentication with the given secret",
		},
		cli.StringFlag{
			Name:  "totp-secret",
			Usage: "Enable TOTP authentication with the given secret",
		},
		cli.StringFlag{
			Name:  "ai-service",
			Usage: "Default AI service to use (e.g., openai, chatglm)",
		},
		cli.StringFlag{
			Name:  "forge",
			Usage: "Default forge/template name to use",
		},
		cli.StringFlag{
			Name:  "review-policy",
			Value: "manual",
			Usage: "Review policy: manual, yolo, or ai",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "Enable debug logging",
		},
		cli.StringFlag{
			Name:  "home",
			Usage: "Set yakit home directory for database",
		},
	},
	Action: func(c *cli.Context) error {
		// Set home directory if specified
		if home := c.String("home"); home != "" {
			os.Setenv("YAKIT_HOME", home)
		}

		// Configure logging
		if c.Bool("debug") {
			log.SetLevel(log.DebugLevel)
		}

		host := c.String("host")
		port := c.Int("port")
		addr := utils.HostPort(host, port)

		log.Infof("Starting AI HTTP Gateway on %s", addr)

		// Create gRPC server (without network listener)
		grpcServer, err := yakgrpc.NewServer(
			yakgrpc.WithInitFacadeServer(false),
		)
		if err != nil {
			return fmt.Errorf("failed to create gRPC server: %w", err)
		}

		// Build gateway options
		opts := []aihttp.GatewayOption{
			aihttp.WithRoutePrefix(c.String("prefix")),
			aihttp.WithGRPCServer(grpcServer),
		}

		// Configure authentication
		if jwtSecret := c.String("jwt-secret"); jwtSecret != "" {
			opts = append(opts, aihttp.WithJWTAuth(jwtSecret))
			log.Info("JWT authentication enabled")
		} else if totpSecret := c.String("totp-secret"); totpSecret != "" {
			opts = append(opts, aihttp.WithTOTP(totpSecret))
			log.Info("TOTP authentication enabled")
			log.Infof("Current TOTP code: %s", aihttp.GetCurrentTOTPCode(totpSecret))
		}

		// Configure default AI settings
		defaultSetting := &ypb.AIStartParams{
			UseDefaultAIConfig: true,
			ReviewPolicy:       c.String("review-policy"),
		}
		if aiService := c.String("ai-service"); aiService != "" {
			defaultSetting.AIService = aiService
		}
		if forge := c.String("forge"); forge != "" {
			defaultSetting.ForgeName = forge
		}
		opts = append(opts, aihttp.WithInitSetting(defaultSetting))

		// Create gateway
		gateway, err := aihttp.NewAIAgentHTTPGateway(opts...)
		if err != nil {
			return fmt.Errorf("failed to create AI HTTP Gateway: %w", err)
		}

		// Create HTTP server
		server := &http.Server{
			Addr:         addr,
			Handler:      gateway.GetHTTPRouteHandler(),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 0, // No timeout for SSE
			IdleTimeout:  120 * time.Second,
		}

		// Start listener
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to listen on %s: %w", addr, err)
		}

		// Handle graceful shutdown
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
			<-sigChan

			log.Info("Shutting down AI HTTP Gateway...")

			shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 10*time.Second)
			defer shutdownCancel()

			gateway.Shutdown(shutdownCtx)
			server.Shutdown(shutdownCtx)
		}()

		// Print startup info
		printStartupInfo(c, addr)

		// Start serving
		log.Infof("AI HTTP Gateway listening on http://%s", addr)
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}

		log.Info("AI HTTP Gateway stopped")
		return nil
	},
}

func printStartupInfo(c *cli.Context, addr string) {
	prefix := c.String("prefix")

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║           AI Agent HTTP Gateway Started                      ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Server:  http://%s%s\n", addr, strings.Repeat(" ", 45-len(addr)))
	fmt.Printf("║  Prefix:  %s%s\n", prefix, strings.Repeat(" ", 52-len(prefix)))
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Endpoints:                                                  ║")
	fmt.Printf("║    GET    %s/setting           - Get settings\n", prefix)
	fmt.Printf("║    POST   %s/setting           - Update settings\n", prefix)
	fmt.Printf("║    POST   %s/run               - Create run\n", prefix)
	fmt.Printf("║    GET    %s/run/{id}          - Get result\n", prefix)
	fmt.Printf("║    GET    %s/run/{id}/events   - SSE stream\n", prefix)
	fmt.Printf("║    POST   %s/run/{id}/events/push - Push event\n", prefix)
	fmt.Printf("║    POST   %s/run/{id}/cancel   - Cancel run\n", prefix)
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	if c.String("jwt-secret") != "" {
		fmt.Println("🔐 JWT Authentication enabled")
		fmt.Println("   Include header: Authorization: Bearer <token>")
		fmt.Println()
	} else if c.String("totp-secret") != "" {
		fmt.Println("🔐 TOTP Authentication enabled")
		fmt.Printf("   Current code: %s\n", aihttp.GetCurrentTOTPCode(c.String("totp-secret")))
		fmt.Println("   Include header: X-TOTP-Code: <code>")
		fmt.Println()
	}
}
