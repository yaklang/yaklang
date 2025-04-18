package js2ssa

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type SSABuilder struct {
	*ssa.PreHandlerInit
}

var Builder ssa.Builder = &SSABuilder{}

func (S SSABuilder) Create() ssa.Builder {
	//TODO implement me
	panic("implement me")
}

func (S SSABuilder) Build(s string, b bool, builder *ssa.FunctionBuilder) error {
	//TODO implement me
	panic("implement me")
}

func (S SSABuilder) FilterFile(s string) bool {
	//TODO implement me
	panic("implement me")
}

func (S SSABuilder) GetLanguage() consts.Language {
	//TODO implement me
	panic("implement me")
}
