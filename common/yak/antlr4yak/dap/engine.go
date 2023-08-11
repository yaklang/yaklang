package dap

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

func (ds *DebugSession) RunProgramInDebugMode(debug bool, program string, args []string) error {
	raw, err := os.ReadFile(program)
	if err != nil {
		return err
	}

	var absPath = program
	if !filepath.IsAbs(absPath) {
		absPath, err = filepath.Abs(absPath)
		if err != nil {
			return errors.Wrap(err, "fetch abs file path failed")
		}
	}

	engine := yak.NewScriptEngine(100)
	if debug {
		engine.SetDebug(true)
		d := NewDAPDebugger()

		// 等待初始化
		d.InitWGAdd()

		// 设置回调
		engine.SetDebugInit(d.Init())
		engine.SetDebugCallback(d.CallBack())

		d.source = &Source{AbsPath: absPath, Name: filepath.Base(absPath)}

		ds.debugger = d
		d.session = ds

		// launch完成
		ds.LaunchWg.Done()
	}

	// inject args in cli
	yaklib.InjectCliArgs(args)

	err = engine.ExecuteMain(string(raw), absPath)
	if err != nil {
		return err
	}

	return nil
}
