package sfvm

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"sync"
)

type SyntaxFlowVirtualMachine[K comparable, V any] struct {
	vars *omap.OrderedMap[string, *omap.OrderedMap[K, V]]

	debug      bool
	frameMutex *sync.Mutex
	frames     []*SFFrame[K, V]
}

func NewSyntaxFlowVirtualMachine[K comparable, V any]() *SyntaxFlowVirtualMachine[K, V] {
	sfv := &SyntaxFlowVirtualMachine[K, V]{
		vars:       omap.NewEmptyOrderedMap[string, *omap.OrderedMap[K, V]](),
		frameMutex: new(sync.Mutex),
	}
	return sfv
}

func (s *SyntaxFlowVirtualMachine[K, V]) Debug(i ...bool) *SyntaxFlowVirtualMachine[K, V] {
	if len(i) > 0 {
		s.debug = i[0]
	} else {
		s.debug = true
	}
	return s
}

func (s *SyntaxFlowVirtualMachine[K, V]) Compile(text string) (ret error) {
	defer func() {
		if err := recover(); err != nil {
			ret = utils.Wrapf(utils.Error(err), "Panic for SyntaxFlow compile")
		}
	}()
	lexer := sf.NewSyntaxFlowLexer(antlr.NewInputStream(text))
	astParser := sf.NewSyntaxFlowParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	result := NewSyntaxFlowVisitor[K, V]()
	result.VisitFlow(astParser.Flow())
	var frame = result.CreateFrame(s.vars)
	s.frames = append(s.frames, frame)
	return nil
}

func (s *SyntaxFlowVirtualMachine[K, V]) Feed(i *omap.OrderedMap[K, V]) {
	s.frameMutex.Lock()
	defer s.frameMutex.Unlock()
	for index, frame := range s.frames {
		err := frame.Debug(s.debug).Execute(i)
		if err != nil {
			log.Errorf("exec frame[%v]: %v\n\t\tCODE: %v", err, index, frame.Text)
		}
	}
}
