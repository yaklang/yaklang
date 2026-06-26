package scannode

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

var DistYakCommand = cli.Command{
	Name:  "distyak",
	Usage: "used by distribution node for running yak file",
	Action: func(c *cli.Context) error {
		ctx, stop := newDistYakContext()
		defer stop()
		runtimeID := os.Getenv("YAK_RUNTIME_ID")
		args := c.Args()
		if len(args) > 0 {
			// args 被解析到了，说明后面跟着文件，去读文件出来吧
			file := args[0]
			if file != "" {
				return runDistYakFile(ctx, file, runtimeID)
			} else {
				return utils.Errorf("empty yak file")
			}
		}

		code := c.String("code")
		return runDistYakCode(ctx, code, runtimeID)
	},
	Flags: []cli.Flag{
		cli.StringFlag{
			Name: "code,c",
		},
	},
	SkipFlagParsing: true,
}

func newDistYakContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func runDistYakFile(parent context.Context, file string, runtimeID string) error {
	var err error
	absFile := file
	if !filepath.IsAbs(absFile) {
		absFile, err = filepath.Abs(absFile)
		if err != nil {
			return utils.Errorf("fetch abs file path failed: %s", err)
		}
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	ctx, cancel := childDistYakContext(parent)
	defer cancel()
	engine := newDistYakEngine(ctx, cancel, runtimeID)
	return engine.ExecuteMainWithContext(ctx, string(raw), absFile)
}

func runDistYakCode(parent context.Context, code string, runtimeID string) error {
	ctx, cancel := childDistYakContext(parent)
	defer cancel()
	engine := newDistYakEngine(ctx, cancel, runtimeID)
	return engine.ExecuteWithContext(ctx, code)
}

func childDistYakContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithCancel(parent)
}

func newDistYakEngine(ctx context.Context, cancel context.CancelFunc, runtimeID string) *yak.ScriptEngine {
	runtimeID = strings.TrimSpace(runtimeID)
	engine := yak.NewScriptEngine(100)
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		yak.BindYakitPluginContextToEngine(engine, yak.CreateYakitPluginContext(runtimeID).
			WithPluginName("distyak").
			WithContext(ctx).
			WithContextCancel(cancel))
		vars := map[string]any{
			"CTX": ctx,
		}
		if runtimeID != "" {
			vars["RUNTIME_ID"] = runtimeID
		}
		engine.SetVars(vars)
		return nil
	})
	return engine
}
