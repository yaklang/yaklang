package yakcmds

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/yakgrpc/aihttp"
)

var AIHTTPGatewayCommand = &cli.Command{
	Name:    "ai-http-gateway",
	Usage:   "Start AI Agent HTTP Gateway (REST/SSE over gRPC)",
	Aliases: []string{"ai-gateway", "aig"},
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "host",
			Value: "0.0.0.0",
			Usage: "HTTP server bind host",
		},
		cli.IntFlag{
			Name:  "port",
			Value: 8089,
			Usage: "HTTP server bind port",
		},
		cli.StringFlag{
			Name:  "prefix",
			Value: "/agent",
			Usage: "route prefix for API endpoints",
		},
		cli.StringFlag{
			Name:  "jwt-secret",
			Usage: "enable JWT auth with this secret",
		},
		cli.StringFlag{
			Name:  "totp-secret",
			Usage: "enable TOTP auth with this secret",
		},
		cli.StringFlag{
			Name:  "home",
			Usage: "home directory for gateway",
		},
		cli.StringFlag{
			Name:  "upload-dir",
			Usage: "directory used to store files uploaded through aihttp",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug logging",
		},
	},
	Action: func(c *cli.Context) error {
		if home := c.String("home"); home != "" {
			os.Setenv("YAKIT_HOME", home)
		}

		var opts []aihttp.GatewayOption

		opts = append(opts,
			aihttp.WithHost(c.String("host")),
			aihttp.WithPort(c.Int("port")),
			aihttp.WithRoutePrefix(c.String("prefix")),
			aihttp.WithDebug(c.Bool("debug")),
			aihttp.WithDatabase(consts.GetGormProjectDatabase()),
		)
		if uploadDir := c.String("upload-dir"); uploadDir != "" {
			opts = append(opts, aihttp.WithUploadDir(uploadDir))
		}

		if secret := c.String("jwt-secret"); secret != "" {
			opts = append(opts, aihttp.WithJWTAuth(secret))
		}
		if secret := c.String("totp-secret"); secret != "" {
			opts = append(opts, aihttp.WithTOTP(secret))
		}

		gw, err := aihttp.NewAIAgentHTTPGateway(opts...)
		if err != nil {
			return fmt.Errorf("create AI HTTP Gateway failed: %w", err)
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigCh
			log.Infof("received signal %v, shutting down...", sig)
			gw.Shutdown()
		}()

		printStartupInfo(c, gw)
		return gw.Start()
	},
}

func printStartupInfo(c *cli.Context, gw *aihttp.AIAgentHTTPGateway) {
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║       AI Agent HTTP Gateway                  ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Printf("  Listening: http://%s\n", gw.GetAddr())
	fmt.Printf("  Prefix:    %s\n", gw.GetRoutePrefix())
	fmt.Printf("  Uploads:   %s\n", gw.GetUploadDir())
	fmt.Println()

	if gw.IsJWTEnabled() {
		token, err := aihttp.GenerateJWTToken(gw.GetJWTSecret(), 24)
		if err == nil {
			fmt.Printf("  JWT Auth:  ENABLED\n")
			fmt.Printf("  Token(24h): %s\n", token)
		}
	}
	if gw.IsTOTPEnabled() {
		code := aihttp.GetCurrentTOTPCode(gw.GetTOTPSecret())
		fmt.Printf("  TOTP Auth: ENABLED\n")
		fmt.Printf("  Current:   %s\n", code)
	}

	fmt.Println()
}
