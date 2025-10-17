package sfvm

import (
	"fmt"
	"sync"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

type SyntaxFlowVirtualMachine struct {
	config *Config

	vars          *omap.OrderedMap[string, ValueOperator]
	compileErrors antlr4util.SourceCodeErrors

	debug      bool
	frameMutex *sync.Mutex
	frames     []*SFFrame
}

func NewSyntaxFlowVirtualMachine(opts ...Option) *SyntaxFlowVirtualMachine {
	config := NewConfig(opts...)
	var vars *omap.OrderedMap[string, ValueOperator]
	if config.initialContextVars != nil {
		vars = config.initialContextVars
	} else {
		vars = omap.NewEmptyOrderedMap[string, ValueOperator]()
	}
	sfv := &SyntaxFlowVirtualMachine{
		vars:       vars,
		frameMutex: new(sync.Mutex),
		config:     config,
	}
	if config.debug {
		sfv.Debug(true)
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
	for _, f := range s.frames {
		f.Show()
	}
}

func (f *SFFrame) Show() {
	fmt.Println("--------------------------")
	for idx, c := range f.Codes {
		fmt.Printf(" %4d| %v\n", idx, c.String())
	}
}

func (s *SyntaxFlowVirtualMachine) ForEachFrame(h func(frame *SFFrame)) {
	for _, i := range s.frames {
		h(i)
	}
}

func (s *SyntaxFlowVirtualMachine) Load(rule *schema.SyntaxFlowRule) (*SFFrame, bool, error) {
	var frame *SFFrame
	opcode, ok := ToOpCodes(rule.OpCodes)
	if ok {
		frame = newSfFrameEx(s.vars, rule.Content, opcode.Opcode, rule, s.config)
		frame.config = s.config
		frame.vm = s
		s.frames = append(s.frames, frame)
		return frame, false, nil
	} else {
		var err error
		frame, err = s.Compile(rule.Content)
		if err != nil {
			return nil, false, utils.Errorf("SyntaxFlow compile error: %v", err)
		}
		// compile only with rule.Content will lose original rule schema info
		// so set it back here
		newFrame := newSfFrameEx(s.vars, rule.Content, frame.Codes, rule, s.config)
		newFrame.config = s.config
		newFrame.vm = s
		return newFrame, true, nil
	}
}

func CompileRule(rule string) (*SFFrame, error) {
	vm := NewSyntaxFlowVirtualMachine()
	frame, err := vm.Compile(rule)
	return frame, err
}

func (s *SyntaxFlowVirtualMachine) Compile(text string) (frame *SFFrame, ret error) {
	if text == "" {
		return nil, utils.Errorf("SyntaxFlow compile error: text is nil")
	}
	defer func() {
		if err := recover(); err != nil {
			ret = utils.Wrapf(utils.Error(err), "Panic for SyntaxFlow compile")
			frame = nil
		}
	}()
	errHandler := antlr4util.SimpleSyntaxErrorHandler(func(msg string, start, end *memedit.Position) {
		s.compileErrors = append(s.compileErrors, antlr4util.NewSourceCodeError(msg, start, end))
	})
	errLis := antlr4util.NewErrorListener(func(self *antlr4util.ErrorListener, recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
		antlr4util.StringSyntaxErrorHandler(self, recognizer, offendingSymbol, line, column, msg, e)
		errHandler(self, recognizer, offendingSymbol, line, column, msg, e)
	})

	lexer := sf.NewSyntaxFlowLexer(antlr.NewInputStream(text))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errLis)
	astParser := sf.NewSyntaxFlowParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	astParser.RemoveErrorListeners()
	astParser.AddErrorListener(errLis)

	result := NewSyntaxFlowVisitor()
	flow := astParser.Flow()
	// fmt.Printf("%v\n", flow.ToStringTree(astParser.RuleNames, astParser))
	if len(errLis.GetErrors()) > 0 {
		return nil, utils.Errorf("SyntaxFlow compile error: %v", errLis.GetErrorString())
	}
	result.rule.Content = text
	result.VisitFlow(flow)
	result.rule.OpCodes = result.ToString()
	frame = result.CreateFrame(s.vars)
	frame.config = s.config
	if len(result.verifyFsInfo) > 0 {
		frame.VerifyFsInfo = result.verifyFsInfo
	}
	frame.rule = result.rule
	frame.vm = s

	s.frames = append(s.frames, frame)

	return frame, nil
}

func (s *SyntaxFlowVirtualMachine) GetErrors() antlr4util.SourceCodeErrors {
	return s.GetCompileErrors()
}

func (s *SyntaxFlowVirtualMachine) GetCompileErrors() antlr4util.SourceCodeErrors {
	return s.compileErrors
}

func (s *SyntaxFlowVirtualMachine) Snapshot() *omap.OrderedMap[string, ValueOperator] {
	s.frameMutex.Lock()
	defer s.frameMutex.Unlock()
	return s.vars.Copy()
}

func (s *SyntaxFlowVirtualMachine) Feed(i ValueOperator) ([]*SFFrameResult, error) {
	s.frameMutex.Lock()
	defer s.frameMutex.Unlock()

	var errs error
	results := make([]*SFFrameResult, 0, len(s.frames))
	for _, frame := range s.frames {
		if res, err := frame.Feed(i); err != nil {
			errs = utils.JoinErrors(errs, err)
		} else {
			results = append(results, res)
		}
	}
	return results, errs
}

func (s *SyntaxFlowVirtualMachine) SetConfig(config *Config) {
	s.config = config
}

func (frame *SFFrame) Feed(i ValueOperator, opt ...Option) (*SFFrameResult, error) {
	for _, o := range opt {
		o(frame.config)
	}
	err := frame.exec(i)
	frame.result.rule = frame.rule
	return frame.result, err
}

func (s *SyntaxFlowVirtualMachine) GetConfig() *Config {
	return s.config
}
