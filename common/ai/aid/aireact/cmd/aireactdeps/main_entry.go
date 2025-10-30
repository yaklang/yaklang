package aireactdeps

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	version = "1.0.0"
)

func init() {
	// 处理中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range sigChan {
			log.Infof("Received interrupt signal: %v, shutting down...", sig)
			os.Exit(0)
		}
	}()
}

func MainEntry() {
	app := cli.NewApp()
	app.Name = "aireact"
	app.Usage = "AI ReAct interactive command line tool"
	app.Version = version

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "lang",
			Value: "zh",
			Usage: "Response language (zh for Chinese, en for English)",
		},
		cli.StringFlag{
			Name:  "query",
			Usage: "One-time query mode (exits after response)",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "Enable debug mode",
		},
		cli.BoolFlag{
			Name:  "no-interact",
			Usage: "Disable interactive tool review mode (auto-approve all tools)",
		},
		cli.BoolFlag{
			Name:  "breakpoint,b",
			Usage: "Enable breakpoint mode (pause before/after each AI interaction for inspection)",
		},
		cli.StringFlag{
			Name:  "file,f",
			Usage: "Monitor a file with traced file context provider (tracks file changes)",
		},
	}

	app.Action = func(c *cli.Context) error {
		return runReActCLI(c)
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}

func runReActCLI(c *cli.Context) error {
	// 初始化配置
	config := &CLIConfig{
		Language:        c.String("lang"),
		Query:           c.String("query"),
		DebugMode:       c.Bool("debug"),
		InteractiveMode: !c.Bool("no-interact"),
		BreakpointMode:  c.Bool("breakpoint"),
		FilePath:        c.String("file"),
	}

	// 设置调试模式
	if config.DebugMode {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug mode enabled")
	}

	if config.BreakpointMode {
		log.Info("Breakpoint mode enabled - will pause before/after each AI interaction")
		log.Info("In breakpoint mode, press Enter/y to continue, e/q to exit, or Ctrl+C to terminate")
	}

	// 显示模式信息
	if config.InteractiveMode {
		log.Info("Interactive tool review mode enabled - will require user approval for each tool use")
	} else {
		log.Info("Non-interactive mode enabled - all tool usage will be automatically approved")
	}

	// 显示文件监控信息
	if config.FilePath != "" {
		log.Infof("File monitoring enabled - will track changes to: %s", config.FilePath)
	}

	log.Info("Starting ReAct CLI Demo")

	// 如果启用了文件监控，显示使用提示
	if config.FilePath != "" {
		log.Info("💡 Tip: File monitoring is active. The AI will see file changes as context.")
		log.Info("💡 You can modify the file and ask questions about the changes!")
	}

	// 初始化数据库和配置
	if err := initializeDatabase(); err != nil {
		log.Errorf("Failed to initialize database: %v", err)
		// Continue anyway, as some features may still work
	}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建 ReAct 应用
	app, err := createReActApp(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create ReAct app: %v", err)
	}

	// 处理初始查询
	if config.Query != "" {
		handleInitialQuery(app.ReactInstance, config.Query)
		time.Sleep(100 * time.Millisecond)
	}

	// 启动交互式循环
	go handleInteractiveLoop(app.ReactInstance, ctx, config)

	// 等待任务完成
	<-ctx.Done()
	log.Info("Context done, shutting down")
	return nil
}

func createReActApp(ctx context.Context, config *CLIConfig) (*ReactApp, error) {
	// 创建 AI 回调
	aiCallback := createAICallback(config)

	// 创建调试 AI 回调包装器
	debugAICallback := createDebugAICallback(aiCallback, config)

	// 创建输入输出通道
	inputChan := make(chan *ypb.AIInputEvent, 100)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	// 创建 ReAct 选项
	reactOptions := buildReActOptions(ctx, debugAICallback, outputChan, config)

	// 创建 ReAct 实例
	reactInstance, err := aireact.NewReAct(reactOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create ReAct instance: %v", err)
	}

	// 启动输入处理器
	go func() {
		for {
			select {
			case inputEvent := <-inputChan:
				if err := reactInstance.SendInputEvent(inputEvent); err != nil {
					log.Errorf("Failed to send input event: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// 启动输出处理器
	go func() {
		for {
			select {
			case event, ok := <-outputChan:
				if !ok {
					return
				}
				if event != nil {
					handleClientEvent(event, inputChan, config.InteractiveMode)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return &ReactApp{
		ReactInstance: reactInstance,
		InputChan:     inputChan,
		OutputChan:    outputChan,
		Config:        config,
	}, nil
}

func createAICallback(config *CLIConfig) aicommon.AICallbackType {
	return aicommon.AIChatToAICallbackType(func(msg string, opts ...aispec.AIConfigOption) (string, error) {
		// 添加流处理器
		opts = append(opts,
			aispec.WithStreamHandler(func(reader io.Reader) {
				showRawStreamOutput(reader, config.BreakpointMode)
			}),
			aispec.WithReasonStreamHandler(func(reader io.Reader) {
				showReasonStreamOutput(reader, config.DebugMode)
			}),
		)

		// 添加超时设置
		opts = append(opts, aispec.WithTimeout(180)) // 3分钟超时
		return ai.Chat(msg, opts...)
	})
}

func createDebugAICallback(aiCallback aicommon.AICallbackType, config *CLIConfig) aicommon.AICallbackType {
	return func(callerConfig aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		if config.DebugMode {
			log.Infof("AI Request: %s", req.GetPrompt())
		}

		// 断点功能 - 在 AI 交互前暂停
		if config.BreakpointMode {
			handleRequestBreakpoint(req.GetPrompt())
		}

		resp, err := aiCallback(callerConfig, req)
		if err != nil {
			if config.DebugMode {
				log.Errorf("AI callback error: %v", err)
			}
			return nil, err
		}

		// 断点功能 - 在 AI 交互后暂停以检查响应
		if config.BreakpointMode {
			setPendingResponse(resp)
		}

		if config.DebugMode {
			log.Infof("AI callback succeeded")
		}
		return resp, nil
	}
}

func buildReActOptions(ctx context.Context, aiCallback aicommon.AICallbackType, outputChan chan<- *schema.AiOutputEvent, config *CLIConfig) []aicommon.ConfigOption {
	options := []aicommon.ConfigOption{
		aicommon.WithContext(ctx),
		aicommon.WithAICallback(aiCallback),
		aicommon.WithDebug(config.DebugMode),
		aicommon.WithMaxIterationCount(50),
		aicommon.WithLanguage(config.Language),
		aicommon.WithTopToolsCount(100),
		aicommon.WithAgreePolicy(aicommon.AgreePolicyAI),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			outputChan <- e
		}),
		aireact.WithBuiltinTools(),
	}

	// 如果指定了文件路径，添加 traced file context provider
	if config.FilePath != "" {
		options = append(options, aicommon.WithTracedFileContext("monitored_file", config.FilePath))
	}

	return options
}
