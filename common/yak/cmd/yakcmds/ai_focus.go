package yakcmds

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/reactloops_yak"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// AIFocusCommand 提供 `yak ai-focus --file xxx.ai-focus.yak --query "..."` 入口，
// 用来快速运行一个 yak 编写的专注模式。
//
// 关键词: yak ai-focus cli, focus mode runner, yak focus command
var AIFocusCommand = &cli.Command{
	Name:    "ai-focus",
	Usage:   "Run a yak focus mode (.ai-focus.yak) with a query",
	Aliases: []string{"aifocus", "focus-yak"},
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:     "file,f",
			Usage:    "path to <name>.ai-focus.yak entry file (sidekick *.yak under the same dir auto-loaded)",
			Required: true,
		},
		cli.StringFlag{
			Name:     "query,q",
			Usage:    "user query to send into the focus mode",
			Required: true,
		},
		cli.IntFlag{
			Name:  "max-iter",
			Value: 0,
			Usage: "override __MAX_ITERATIONS__ when > 0",
		},
		cli.BoolFlag{
			Name:  "json",
			Usage: "emit AI output events as JSON lines instead of human-readable text",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug log",
		},
		cli.IntFlag{
			Name:  "timeout",
			Value: 600,
			Usage: "max seconds to wait for the focus mode task to complete",
		},
		cli.StringFlag{
			Name:  "ai-type",
			Value: "",
			Usage: "AI provider type override (e.g. chatglm / openai); empty means use default",
		},
	},
	Action: runAIFocusCLI,
}

func runAIFocusCLI(c *cli.Context) error {
	mainPath := c.String("file")
	query := c.String("query")
	useJSON := c.Bool("json")
	if c.Bool("debug") {
		log.SetLevel(log.DebugLevel)
	}
	if mainPath == "" {
		return utils.Error("--file is required")
	}
	if query == "" {
		return utils.Error("--query is required")
	}

	// 1. 校验 + 加载 + 注册（拆出去方便单元测试）
	focusName, err := PrepareYakAIFocusFromFile(mainPath)
	if err != nil {
		return err
	}

	// 3. 准备 ctx + signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timeoutSec := c.Int("timeout")
	if timeoutSec > 0 {
		var cancelTimeout context.CancelFunc
		ctx, cancelTimeout = context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
		defer cancelTimeout()
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			log.Info("yak ai-focus: interrupt received, cancelling")
			cancel()
		case <-ctx.Done():
		}
	}()

	// 4. AI 回调
	aiCallback := buildAIFocusAICallback(c.String("ai-type"))

	// 5. EventHandler：流到 stdout，并监控 task 状态
	taskDone := make(chan struct{})
	var doneOnce atomic.Bool
	signalDone := func() {
		if doneOnce.CompareAndSwap(false, true) {
			close(taskDone)
		}
	}
	eventHandler := func(e *schema.AiOutputEvent) {
		if e == nil {
			return
		}
		printAIFocusEvent(e, useJSON)
		// 监听 react_task_status_changed → completed/failed → 退出
		if e.NodeId == "react_task_status_changed" && len(e.Content) > 0 {
			var data map[string]any
			if err := json.Unmarshal(e.Content, &data); err == nil {
				status := utils.InterfaceToString(data["react_task_now_status"])
				switch status {
				case "completed", "failed", "cancelled", "aborted":
					log.Infof("yak ai-focus: task status=%s, will exit", status)
					signalDone()
				}
			}
		}
	}

	// 6. 构造 ReAct
	reactOptions := []aicommon.ConfigOption{
		aicommon.WithContext(ctx),
		aicommon.WithAICallback(aiCallback),
		aicommon.WithDebug(c.Bool("debug")),
		aicommon.WithLanguage("zh"),
		aicommon.WithTopToolsCount(100),
		aicommon.WithAgreePolicy(aicommon.AgreePolicyAI),
		aicommon.WithEventHandler(eventHandler),
		aireact.WithBuiltinTools(),
	}
	if maxIter := c.Int("max-iter"); maxIter > 0 {
		reactOptions = append(reactOptions, aicommon.WithMaxIterationCount(int64(maxIter)))
	}

	reactInstance, err := aireact.NewReAct(reactOptions...)
	if err != nil {
		return utils.Wrapf(err, "create ReAct instance")
	}

	// 7. 发送 free input + focus mode 锁定
	inputEvent := &ypb.AIInputEvent{
		IsFreeInput:   true,
		FreeInput:     query,
		FocusModeLoop: focusName,
	}
	if err := reactInstance.SendInputEvent(inputEvent); err != nil {
		return utils.Wrapf(err, "send focus mode input event")
	}
	log.Infof("yak ai-focus: query dispatched (focus=%s)", focusName)

	// 8. 等任务完成 / ctx 超时 / 信号
	select {
	case <-taskDone:
		log.Info("yak ai-focus: task done")
	case <-ctx.Done():
		log.Infof("yak ai-focus: context done: %v", ctx.Err())
	}
	return nil
}

// PrepareYakAIFocusFromFile 把 *.ai-focus.yak 文件加载、收集 sidekick、注册到 reactloops，
// 并返回最终的 focus mode 名称。
//
// 这一层从 runAIFocusCLI 拆出来，是为了让 e2e 测试能够独立验证 CLI 的"文件 → 注册"链路，
// 而不必启动整个 ReAct 运行时与 AI 回调。
//
// 关键词: yak ai-focus prepare, focus file to registry
func PrepareYakAIFocusFromFile(mainPath string) (string, error) {
	if mainPath == "" {
		return "", utils.Error("--file is required")
	}
	abs, err := filepath.Abs(mainPath)
	if err != nil {
		return "", utils.Wrapf(err, "resolve absolute path of %s", mainPath)
	}
	if _, err := os.Stat(abs); err != nil {
		return "", utils.Wrapf(err, "stat focus file %s", abs)
	}
	if !strings.HasSuffix(abs, reactloops.FocusModeFileSuffix) {
		return "", utils.Errorf("file must end with %s, got %s", reactloops.FocusModeFileSuffix, abs)
	}

	bundle, err := reactloops_yak.LoadSingleFile(abs)
	if err != nil {
		return "", utils.Wrapf(err, "load focus mode bundle from %s", abs)
	}
	log.Infof("yak ai-focus loaded bundle name=%s entry=%s sidekicks=%d",
		bundle.Name, bundle.EntryFile, len(bundle.Sidekicks))

	if err := reactloops.RegisterYakFocusModeFromBundle(bundle); err != nil {
		return "", utils.Wrapf(err, "register yak focus mode %s", bundle.Name)
	}
	return bundle.Name, nil
}

// buildAIFocusAICallback 构造一个使用 ai.Chat 的 AI 回调，并把流式输出写到 stderr。
// 关键词: yak ai-focus ai callback, ai.Chat wrapper
func buildAIFocusAICallback(aiType string) aicommon.AICallbackType {
	return aicommon.AIChatToAICallbackType(func(msg string, opts ...aispec.AIConfigOption) (string, error) {
		if aiType != "" {
			opts = append(opts, aispec.WithType(aiType))
		}
		opts = append(opts,
			aispec.WithStreamHandler(func(reader io.Reader) {
				_, _ = io.Copy(os.Stderr, reader)
			}),
			aispec.WithTimeout(180),
		)
		return ai.Chat(msg, opts...)
	})
}

// printAIFocusEvent 把单个 AiOutputEvent 输出到 stdout。
// json 模式：每行一个 JSON；text 模式：节点 ID + 类型 + 内容预览。
func printAIFocusEvent(e *schema.AiOutputEvent, useJSON bool) {
	if useJSON {
		raw, err := json.Marshal(e)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal event failed: %v\n", err)
			return
		}
		fmt.Fprintln(os.Stdout, string(raw))
		return
	}
	// 人类可读模式：跳过纯心跳/状态噪声
	switch e.Type {
	case schema.EVENT_TYPE_CONSUMPTION, schema.EVENT_TYPE_PONG, schema.EVENT_TYPE_PRESSURE,
		schema.EVENT_TYPE_AI_FIRST_BYTE_COST_MS, schema.EVENT_TYPE_AI_TOTAL_COST_MS:
		return
	}
	prefix := fmt.Sprintf("[%s]", e.Type)
	if e.NodeId != "" {
		prefix += fmt.Sprintf("[%s]", e.NodeId)
	}
	if e.IsStream && len(e.StreamDelta) > 0 {
		fmt.Fprintf(os.Stdout, "%s delta: %s\n", prefix, string(e.StreamDelta))
		return
	}
	if len(e.Content) > 0 {
		fmt.Fprintf(os.Stdout, "%s %s\n", prefix, string(e.Content))
		return
	}
	fmt.Fprintln(os.Stdout, prefix)
}
