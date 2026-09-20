//go:build irify_exclude

package yakcmds

import cli "github.com/yaklang/yaklang/common/urfavecli"

// Keep the command group available to the shared entrypoint without linking
// compiler/scan commands that require the full edition's language builders.
var SSACompilerCommands = []*cli.Command{}
