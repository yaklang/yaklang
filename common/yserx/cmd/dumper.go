package main

import (
	"bufio"
	"bytes"
	"github.com/urfave/cli"
	"os"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/yak/yaklib/codec"
	"yaklang.io/yaklang/common/yserx"
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
