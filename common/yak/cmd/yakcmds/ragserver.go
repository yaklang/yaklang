package yakcmds

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/yaklang/yaklang/common/log"
	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/airaghttp"
)

// RAGServerCommands RAG 知识库 HTTP 服务相关命令
// 工作流: rag-list 查看在线库 -> rag-download 下载导入 -> rag-server 启动
// 关键词: rag-list, rag-download, rag-server, airaghttp
var RAGServerCommands = []*cli.Command{
	ragListCommand,
	ragDownloadCommand,
	ragServerCommand,
}

// rag-list 查看现有可直接下载的知识库
var ragListCommand = &cli.Command{
	Name:  "rag-list",
	Usage: "List downloadable RAG knowledge bases from the online registry",
	Action: func(c *cli.Context) error {
		infos, err := airaghttp.ListOnlineRags()
		if err != nil {
			return fmt.Errorf("list online rags failed: %w", err)
		}
		if len(infos) == 0 {
			fmt.Println("no online rag available")
			return nil
		}

		fmt.Printf("%-28s %-22s %-10s %-12s %s\n", "NAME", "NAME_ZH", "VERSION", "SIZE", "FILE")
		fmt.Println(strings.Repeat("-", 100))
		for _, info := range infos {
			fmt.Printf("%-28s %-22s %-10s %-12s %s\n",
				truncate(info.Name, 28),
				truncate(info.NameZh, 22),
				truncate(info.Version, 10),
				humanizeSize(info.FileSize),
				info.File,
			)
		}
		fmt.Printf("\ntotal: %d knowledge base(s)\n", len(infos))
		fmt.Println("download via: yak rag-download --name <NAME>  (or --all)")
		return nil
	},
}

// rag-download 下载并导入知识库到 profile 数据库
var ragDownloadCommand = &cli.Command{
	Name:  "rag-download",
	Usage: "Download a RAG knowledge base and import it into the local profile database",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "name",
			Usage: "knowledge base name (english or chinese name from rag-list)",
		},
		cli.BoolFlag{
			Name:  "all",
			Usage: "download all available knowledge bases",
		},
		cli.BoolFlag{
			Name:  "force",
			Usage: "force re-download and overwrite existing data",
		},
		cli.StringFlag{
			Name:  "output-dir",
			Usage: "download directory (default: ~/yakit-projects/libs)",
		},
	},
	Action: func(c *cli.Context) error {
		name := c.String("name")
		all := c.Bool("all")
		force := c.Bool("force")
		outputDir := c.String("output-dir")

		if !all && name == "" {
			return utils.Error("either --name or --all is required")
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			log.Info("received interrupt signal, cancelling download...")
			cancel()
		}()

		onProgress := func(ragName string, percent float64, message string) {
			log.Infof("[%s %.0f%%] %s", ragName, percent, message)
		}

		if all {
			if err := airaghttp.DownloadAllRags(ctx, force, outputDir, onProgress); err != nil {
				return fmt.Errorf("download all rags failed: %w", err)
			}
			fmt.Println("all knowledge bases downloaded and imported")
			return nil
		}

		if err := airaghttp.DownloadRag(ctx, name, force, outputDir, onProgress); err != nil {
			return fmt.Errorf("download rag failed: %w", err)
		}
		fmt.Printf("knowledge base %q downloaded and imported\n", name)
		return nil
	},
}

// rag-server 启动 RAG 知识库 HTTP 服务
var ragServerCommand = &cli.Command{
	Name:  "rag-server",
	Usage: "Start the standalone RAG knowledge base HTTP server (search only, no admin)",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "config",
			Usage: "path to rag-server.yaml (optional; uses default config if omitted)",
		},
		cli.StringFlag{
			Name:  "host",
			Usage: "HTTP server bind host (override config)",
		},
		cli.IntFlag{
			Name:  "port",
			Usage: "HTTP server bind port (override config)",
		},
		cli.StringFlag{
			Name:  "prefix",
			Usage: "route prefix (override config, default /api/rag-server)",
		},
		cli.StringFlag{
			Name:  "auth-token",
			Usage: "bearer auth token (override config; empty means no auth)",
		},
		cli.StringFlag{
			Name:  "rag-files",
			Usage: "comma-separated local .rag files to import on startup (override config)",
		},
		cli.IntFlag{
			Name:  "concurrent",
			Usage: "max simultaneous chat requests (override config)",
		},
		cli.StringFlag{
			Name:  "ai-tier",
			Value: "basic",
			Usage: "model tier when using global tiered aiconfig: basic (lightweight, default) or standard (intelligent)",
		},
		cli.StringFlag{
			Name:  "ai-type",
			Usage: "AI service type (override config)",
		},
		cli.StringFlag{
			Name:  "ai-model",
			Usage: "AI model name (override config; non-empty switches to single callback mode)",
		},
		cli.StringFlag{
			Name:  "ai-apikey",
			Usage: "AI API key (override config)",
		},
		cli.StringFlag{
			Name:  "ai-domain",
			Usage: "AI service domain (override config)",
		},
		cli.BoolTFlag{
			Name:  "fe",
			Usage: "serve the built-in read-only search web UI at the root path (default: on; use --fe=false to disable)",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug logging",
		},
	},
	Action: func(c *cli.Context) error {
		var config *airaghttp.RAGServerConfig
		if cfgPath := c.String("config"); cfgPath != "" {
			loaded, err := airaghttp.LoadConfigFromFile(cfgPath)
			if err != nil {
				return fmt.Errorf("load config failed: %w", err)
			}
			config = loaded
			log.Infof("loaded config from %s", cfgPath)
		} else {
			config = airaghttp.NewDefaultConfig()
			log.Info("no --config provided, using default config")
		}

		opts := []airaghttp.Option{
			airaghttp.WithHost(c.String("host")),
			airaghttp.WithPort(c.Int("port")),
			airaghttp.WithRoutePrefix(c.String("prefix")),
			airaghttp.WithAuthToken(c.String("auth-token")),
			airaghttp.WithConcurrent(c.Int("concurrent")),
			airaghttp.WithAIService(c.String("ai-type"), c.String("ai-model"), c.String("ai-apikey"), c.String("ai-domain")),
			airaghttp.WithAITier(c.String("ai-tier")),
			airaghttp.WithServeFrontend(c.Bool("fe")),
			airaghttp.WithDebug(c.Bool("debug")),
		}
		if ragFiles := c.String("rag-files"); ragFiles != "" {
			files := make([]string, 0)
			for _, f := range strings.Split(ragFiles, ",") {
				if trimmed := strings.TrimSpace(f); trimmed != "" {
					files = append(files, trimmed)
				}
			}
			opts = append(opts, airaghttp.WithRagFiles(files...))
		}

		server, err := airaghttp.NewRAGHTTPServer(config, opts...)
		if err != nil {
			return fmt.Errorf("create rag http server failed: %w", err)
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigCh
			log.Infof("received signal %v, shutting down...", sig)
			server.Shutdown()
		}()

		printRAGServerStartupInfo(server)
		return server.Start()
	},
}

func printRAGServerStartupInfo(server *airaghttp.RAGHTTPServer) {
	addr := server.GetAddr()
	prefix := server.GetRoutePrefix()
	fmt.Println("============================================")
	fmt.Println("           RAG Knowledge Base Server")
	fmt.Println("============================================")
	fmt.Printf("  Listening:    http://%s\n", addr)
	fmt.Printf("  Prefix:       %s\n", prefix)
	fmt.Printf("  Health:       http://%s%s/health\n", addr, prefix)
	fmt.Printf("  Collections:  http://%s%s/collections\n", addr, prefix)
	fmt.Printf("  Search:       http://%s%s/search\n", addr, prefix)
	fmt.Printf("  Chat (SSE):   http://%s%s/chat?q=...\n", addr, prefix)
	if server.IsFrontendEnabled() {
		fmt.Printf("  Web UI:       http://%s/   (built-in read-only search page)\n", addr)
	}
	fmt.Printf("  Ready KBs:    %d\n", len(server.GetReadyCollections()))
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop.")
	fmt.Println()
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func humanizeSize(size int64) string {
	if size <= 0 {
		return "unknown"
	}
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%dB", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f%cB", float64(size)/float64(div), "KMGTPE"[exp])
}
