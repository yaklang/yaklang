package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils/cli"
)

func init() {
	args := append([]string{}, os.Args...)
	cli.InjectCliArgs(args)
	if len(args) > 1 {
		cli.DefaultCliApp.SetArgs(append([]string{}, args[1:]...))
	} else {
		cli.DefaultCliApp.SetArgs(nil)
	}

	name := filepath.Base(os.Args[0])
	name = strings.TrimSuffix(name, filepath.Ext(name))
	cli.DefaultCliApp.SetCliName(name)
}
