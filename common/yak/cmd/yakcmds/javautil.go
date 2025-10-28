//go:build !no_language
// +build !no_language

package yakcmds

import (
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yserx"
)

var JavaUtils = []*cli.Command{
	{
		Name:    "serialdumper",
		Usage:   "Java SerialDumper in Yaklang/Golang Implemented",
		Aliases: []string{"sd"},
		Action: func(c *cli.Context) {
			if len(c.Args()) > 0 {
				raw, err := codec.DecodeHex(c.Args()[0])
				if err != nil {
					log.Error(err)
					return
				}
				d := yserx.JavaSerializedDumper(raw)
				println(d)
			}
		},
	},
	JavaDecompilerCommand,
	JavaDecompilerSelfChecking,
}
