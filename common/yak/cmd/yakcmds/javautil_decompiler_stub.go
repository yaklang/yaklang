//go:build irify_exclude

package yakcmds

import (
	"fmt"

	"github.com/yaklang/yaklang/common/urfavecli"
)

func javaDecompilerCommands() []*cli.Command {
	unavailable := func(c *cli.Context) error {
		return fmt.Errorf("java decompiler is not available in yak-slim (irify_exclude) builds")
	}
	return []*cli.Command{
		{
			Name:    "java-decompiler-self-checking",
			Aliases: []string{"jdsc"},
			Usage:   "Java decompiler (unavailable in yak-slim builds)",
			Action:  unavailable,
		},
		{
			Name:    "java-decompiler",
			Aliases: []string{"jd"},
			Usage:   "Java decompiler (unavailable in yak-slim builds)",
			Action:  unavailable,
		},
	}
}
