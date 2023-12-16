package sfvm

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"reflect"
	"sync"
)

type SyntaxFlowVirtualMachine[V any] struct {
	vars *omap.OrderedMap[string, *omap.OrderedMap[string, V]]

	debug      bool
	frameMutex *sync.Mutex
	frames     []*SFFrame[V]
}

func NewSyntaxFlowVirtualMachine[V any]() *SyntaxFlowVirtualMachine[V] {
	sfv := &SyntaxFlowVirtualMachine[V]{
		vars:       omap.NewEmptyOrderedMap[string, *omap.OrderedMap[string, V]](),
		frameMutex: new(sync.Mutex),
	}
	return sfv
}

func (s *SyntaxFlowVirtualMachine[V]) Debug(i ...bool) *SyntaxFlowVirtualMachine[V] {
	if len(i) > 0 {
		s.debug = i[0]
	} else {
		s.debug = true
	}
	return s
}

func (s *SyntaxFlowVirtualMachine[V]) Compile(text string) (ret error) {
	defer func() {
		if err := recover(); err != nil {
			ret = utils.Wrapf(utils.Error(err), "Panic for SyntaxFlow compile")
		}
	}()
	lexer := sf.NewSyntaxFlowLexer(antlr.NewInputStream(text))
	astParser := sf.NewSyntaxFlowParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	result := NewSyntaxFlowVisitor[V]()
	result.text = text
	result.VisitFlow(astParser.Flow())
	var frame = result.CreateFrame(s.vars)
	s.frames = append(s.frames, frame)
	return nil
}

func (s *SyntaxFlowVirtualMachine[V]) Feed(i *omap.OrderedMap[string, V]) *omap.OrderedMap[string, any] {
	s.frameMutex.Lock()
	defer s.frameMutex.Unlock()

	result := omap.NewOrderedMap(map[string]any{})
	for index, frame := range s.frames {
		err := frame.Debug(s.debug).exec(i)
		if err != nil {
			log.Errorf("exec frame[%v]: %v\n\t\tCODE: %v", err, index, frame.Text)
		}
		for i := 0; i < frame.stack.Size(); i++ {
			v := frame.stack.Pop()
			if v.IsMap() {
				v.AsMap().Map(func(s string, v V) (string, V, error) {
					err := result.Add(reflect.ValueOf(v).Interface())
					if err != nil {
						log.Errorf("pop frame result failed: %v", err)
					}
					return s, v, nil
				})
			} else {
				err := result.Add(v.Value())
				if err != nil {
					log.Warnf("pop frame result failed: %v", err)
				}
			}
		}
	}
	return result
}
