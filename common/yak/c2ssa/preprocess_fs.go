package c2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/c2ssa/preprocess"
)

func newCPreprocessFS(underlying fi.FileSystem) fi.FileSystem {
	project := preprocess.BuildProject(underlying, preprocess.DefaultConfig())
	hookFS := filesys.NewHookFS(underlying)
	hookFS.AddReadHook(&filesys.ReadHook{
		Matcher: filesys.SuffixMatcher(".c"),
		AfterRead: func(ctx *filesys.ReadHookContext, data []byte) ([]byte, error) {
			src := string(data)
			out, err := project.PreprocessTU(ctx.Name, src)
			if err != nil {
				log.Warnf("C preprocess failed for %s: %v, using original source", ctx.Name, err)
				return data, nil
			}
			return []byte(out), nil
		},
	})
	return hookFS
}
