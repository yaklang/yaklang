package yakcmds

import (
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yserx"
)

var JavaUtils = []cli.Command{
	{Name: "serialdumper", Aliases: []string{"sd"}, Action: func(c *cli.Context) {
		if len(c.Args()) > 0 {
			raw, err := codec.DecodeHex(c.Args()[0])
			if err != nil {
				log.Error(err)
				return
			}
			d := yserx.JavaSerializedDumper(raw)
			println(d)
		}
	}},
}

func init() {
	for _, i := range JavaUtils {
		i.Category = "Java Serialization Utils"
	}
}
