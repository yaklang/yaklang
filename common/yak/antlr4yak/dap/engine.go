package dap

import (
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

func RunProgramInDebugMode(debug bool, program string, args []string) error {
	raw, err := os.ReadFile(program)
	if err != nil {
		return err
	}

	var absPath = program
	if !filepath.IsAbs(absPath) {
		absPath, err = filepath.Abs(absPath)
		if err != nil {
			return utils.Errorf("fetch abs file path failed: %s", err)
		}
	}

	engine := yak.NewScriptEngine(100)
	if debug {
		engine.SetDebug(true)
		d := NewDAPDebugger()

		// 等待初始化
		d.InitWGAdd()

		engine.SetDebugInit(d.Init())
		engine.SetDebugCallback(d.CallBack())

		ds.debugger = d
		d.session = ds
	}

	// inject args in cli
	yaklib.InjectCliArgs(args)

	err = engine.ExecuteMain(string(raw), absPath)
	if err != nil {
		return err
	}

	return nil
}
