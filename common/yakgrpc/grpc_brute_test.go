package yakgrpc

import (
	"bytes"
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"strings"
	"testing"
	"time"
)

func TestGRPCMUSTPASS_Brute(t *testing.T) {
	redisPasswd := "123456"
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	_ = cancel
	host, port := tools.DebugMockRedis(ctx, true, redisPasswd)
	target := utils.HostPort(host, port)

	host, port = tools.DebugMockRedis(ctx, false)
	unAuthTarget := utils.HostPort(host, port)
	weakPasswdOk := false
	unAuthOk := false
	feedbackClient := yaklib.NewVirtualYakitClient(func(result *ypb.ExecResult) error {
		spew.Dump(result)

		if result.IsMessage {
			if bytes.Contains(result.GetMessage(), []byte("Weak Password[redis]")) {
				weakPasswdOk = true
			}
			if bytes.Contains(result.GetMessage(), []byte("未授权访问")) {
				unAuthOk = true
			}
		}
		return nil
	})

	targetFile, err := utils.DumpHostFileWithTextAndFiles(target+"\n"+unAuthTarget, "\n")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(targetFile)
	userListFile, err := utils.DumpFileWithTextAndFiles(strings.Join([]string{}, "\n"), "\n")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(userListFile)

	passListFile, err := utils.DumpFileWithTextAndFiles(strings.Join([]string{"123456"}, "\n"), "\n")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(passListFile)
	cliArgs := []string{
		"yak",
		"--types", "redis",
		"--target-file", targetFile,
		"--user-list-file", userListFile,
		"--pass-list-file", passListFile,
		"--replace-default-password-dict",
		"--replace-default-username-dict",
		"--ok-to-stop",
		"--concurrent", "50",
		"--task-concurrent", "1",
		"--delay-min", "1",
		"--delay-max", "5",
	}

	engine := yak.NewYakitVirtualClientScriptEngine(feedbackClient)
	log.Infof("engine.ExecuteExWithContext(stream.Context(), debugScript ... \n")
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		yak.BindYakitPluginContextToEngine(engine, &yak.YakitPluginContext{
			PluginName: "brute-temp",
			Ctx:        ctx,
		})
		yak.HookCliArgs(engine, cliArgs)
		return nil
	})
	_, err = engine.ExecuteExWithContext(ctx, startBruteScript, map[string]any{
		"CTX":         ctx,
		"PLUGIN_NAME": "brute-temp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !weakPasswdOk {
		t.Fatal("brute weak password failed")
	}
	if !unAuthOk {
		t.Fatal("brute unAuth failed")
	}
}
