package yakcmds

import (
	"strings"
	"testing"

	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/yak/distyak"
)

func TestRetiredNodeCommand(t *testing.T) {
	for _, args := range [][]string{{"yak", "node"}, {"yak", "node", "--api-url", "http://127.0.0.1:1", "--enrollment-token", "unused"}} {
		app := cli.NewApp()
		// Observe the error without the CLI's process-exiting default handler.
		app.ExitErrHandler = func(*cli.Context, error) {}
		for _, command := range DistributionCommands {
			app.Commands = append(app.Commands, *command)
		}
		err := app.Run(args)
		exit, ok := err.(cli.ExitCoder)
		if !ok || exit.ExitCode() != 1 || !strings.Contains(err.Error(), "Legion-distributed yaklang-node") {
			t.Fatalf("retired node command must explain migration and fail: %v", err)
		}
	}
	if DistributionCommands[0] != &distyak.DistYakCommand {
		t.Fatal("distyak must use the public engine entrypoint directly")
	}
}
