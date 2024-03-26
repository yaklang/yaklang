package ssareducer

import (
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"os"
	"strings"
)

type ReducerCompiler struct {
	base   string
	config *Config
}

func NewReducerCompiler(base string, opts ...Option) *ReducerCompiler {
	c := NewConfig()
	for _, opt := range opts {
		opt(c)
	}
	return &ReducerCompiler{
		base:   base,
		config: c,
	}
}

func (r *ReducerCompiler) Compile() error {
	var opts []filesys.Option
	if r.config.embedFS != nil {
		opts = append(opts, filesys.WithEmbedFS(*r.config.embedFS))
	}

	var visited = filter.NewFilter()
	defer visited.Close()

	c := r.config
	opts = append(opts, filesys.WithFileStat(func(pathname string, info os.FileInfo) error {
		if len(c.exts) > 0 {
			skipped := true
			for _, ext := range c.exts {
				if strings.HasSuffix(strings.ToLower(pathname), strings.ToLower(ext)) {
					skipped = false
					break
				}
			}
			if skipped {
				return nil
			}
		}

		if visited.Exist(pathname) {
			return nil
		}
		if r.config.compileMethods == nil {
			return utils.Errorf("Compile method is nil for lib: %v", r.base)
		}

		results, err := r.config.compileMethods(r, pathname)
		if err != nil {
			if r.config.stopAtCompileError {
				return err
			}
			log.Warnf("Compile error: %v", err)
		}
		for _, result := range results {
			visited.Insert(result)
		}
		return nil
	}))

	err := filesys.Recursive(r.base, opts...)
	if err != nil {
		return err
	}
	return nil
}
