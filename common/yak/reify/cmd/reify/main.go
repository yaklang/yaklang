package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

type inspectionReport struct {
	Scope    string           `json:"scope"`
	Artifact *core.Artifact   `json:"artifact"`
	Parser   *core.ParserInfo `json:"parser,omitempty"`
}

type checkReport struct {
	Scope string              `json:"scope"`
	Check *loader.CheckResult `json:"check"`
}

type liftReport struct {
	Scope         string              `json:"scope"`
	Mode          string              `json:"mode"`
	Region        core.Region         `json:"region"`
	Assembly      string              `json:"assembly,omitempty"`
	Instructions  int                 `json:"instructions,omitempty"`
	IR            *core.LowIR         `json:"ir,omitempty"`
	SSAIR         string              `json:"ssair,omitempty"`
	Source        string              `json:"source,omitempty"`
	Diagnostics   []core.Diagnostic   `json:"diagnostics,omitempty"`
	BoundaryStats *core.BoundaryStats `json:"boundary_stats,omitempty"`
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	app := newCLIApp(&commandEnv{ctx: ctx, stdout: stdout, stderr: stderr})
	err := app.Run(append([]string{"reify"}, args...))
	if err == nil {
		return 0
	}
	code := 1
	if exitErr, ok := err.(cli.ExitCoder); ok {
		code = exitErr.ExitCode()
	}
	if err.Error() != "" {
		fmt.Fprintf(stderr, "reify: %v\n", err)
	}
	return code
}
