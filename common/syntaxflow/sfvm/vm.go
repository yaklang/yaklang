package sfvm

import (
	"fmt"
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"sync"
)

type SyntaxFlowVirtualMachine struct {
	vars *omap.OrderedMap[string, any]

	debug      bool
	frameMutex *sync.Mutex
	frames     []*SFFrame
}

func NewSyntaxFlowVirtualMachine() *SyntaxFlowVirtualMachine {
	sfv := &SyntaxFlowVirtualMachine{
		vars:       omap.NewEmptyOrderedMap[string, any](),
		frameMutex: new(sync.Mutex),
	}
	return sfv
}

func (s *SyntaxFlowVirtualMachine) Debug(i ...bool) *SyntaxFlowVirtualMachine {
	if len(i) > 0 {
		s.debug = i[0]
	} else {
		s.debug = true
	}
	return s
}

func (s *SyntaxFlowVirtualMachine) Show() {
	for _, i := range s.frames {
		for _, c := range i.Codes {
			fmt.Println(c.String())
		}
	}
}

func (s *SyntaxFlowVirtualMachine) ForEachFrame(h func(frame *SFFrame)) {
	for _, i := range s.frames {
		h(i)
	}
}

func (s *SyntaxFlowVirtualMachine) Compile(text string) (ret error) {
	defer func() {
		if err := recover(); err != nil {
			ret = utils.Wrapf(utils.Error(err), "Panic for SyntaxFlow compile")
		}
	}()
	errLis := antlr4util.NewErrorListener()

	lexer := sf.NewSyntaxFlowLexer(antlr.NewInputStream(text))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errLis)
	astParser := sf.NewSyntaxFlowParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	astParser.RemoveErrorListeners()
	astParser.AddErrorListener(errLis)

	result := NewSyntaxFlowVisitor()
	result.text = text
	result.VisitFlow(astParser.Flow())
	var frame = result.CreateFrame(s.vars)
	s.frames = append(s.frames, frame)

	if len(errLis.GetErrors()) > 0 {
		return utils.Errorf("SyntaxFlow compile error: %v", errLis.GetErrors())
	}
	return nil
}

func (s *SyntaxFlowVirtualMachine) Feed(i *omap.OrderedMap[string, any]) *omap.OrderedMap[string, any] {
	s.frameMutex.Lock()
	defer s.frameMutex.Unlock()

	result := omap.NewOrderedMap(map[string]any{})
	for index, frame := range s.frames {
		err := frame.Debug(s.debug).exec(i)
		if err != nil {
			log.Errorf("exec frame[%v]: %v\n\t\tCODE: %v", err, index, frame.Text)
		}
		if frame.stack.Len() > 1 {
			log.Infof("stack unbalanced: %v", frame.stack.Len())
		}
	}
	s.vars.Map(func(s string, a any) (string, any, error) {
		result.Set(s, a)
		return s, a, nil
	})
	return result
}
