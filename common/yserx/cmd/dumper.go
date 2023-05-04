package main

import (
	"bufio"
	"bytes"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yserx"
	"os"
)

func main() {
	var err error
	app := cli.NewApp()

	app.Action = func(c *cli.Context) error {
		hexRaw := c.Args().First()
		raw, err := codec.DecodeHex(hexRaw)
		if err != nil {
			return err
		}

		_, err = yserx.ParseJavaSerializedEx(bufio.NewReader(bytes.NewBuffer(raw)), os.Stdout)
		if err != nil {
			return err
		}
		return nil
	}

	err = app.Run(os.Args)
	if err != nil {
		log.Error(err)
		return
	}
}
