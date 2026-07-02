package main

import (
	"bytes"
	"encoding/gob"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yakdoc/overviewutil"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func main() {
	if len(os.Args) < 2 {
		return
	}

	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())

	// 把 overviews/<lib>.md 首段烤进 ScriptLib.OverviewShort, 让运行时(AI loop)能零成本
	// 拼"库选择索引"。overviews 仍是唯一作者源, 仅增量几 KB 进 doc.gob.zst, 不新增 embed。
	// 关键词: OverviewShort 注入, overviews 首段, 库选择索引数据源
	injectOverviewShort(helper)

	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(&helper); err != nil {
		panic(err)
	}

	if newBuf, err := utils.ZstdCompress(buf.Bytes()); err != nil {
		panic(err)
	} else if err = ioutil.WriteFile(os.Args[1], newBuf, 0o666); err != nil {
		panic(err)
	} else {
	}
}

// injectOverviewShort 从 overviews 目录派生每个库的一句话定位, 写入 helper.Libs[*].OverviewShort。
// overviews 目录用 yakdoc.GetProjectPath() 定位, 不依赖 go run 的当前工作目录。
func injectOverviewShort(helper *yakdoc.DocumentHelper) {
	if helper == nil || len(helper.Libs) == 0 {
		return
	}
	overviewsDir := filepath.Join(yakdoc.GetProjectPath(), "common", "yak", "yakdoc", "generate_web_doc", "overviews")
	overviews := overviewutil.LoadAll(overviewsDir)
	if len(overviews) == 0 {
		log.Warnf("no overviews loaded from %s; OverviewShort will be empty", overviewsDir)
		return
	}
	injected := 0
	for name, lib := range helper.Libs {
		if lib == nil {
			continue
		}
		full, ok := overviews[name]
		if !ok {
			continue
		}
		short := overviewutil.FirstParagraph(full)
		if short != "" {
			lib.OverviewShort = short
			injected++
		}
	}
	log.Infof("injected OverviewShort into %d/%d libraries", injected, len(helper.Libs))
}
