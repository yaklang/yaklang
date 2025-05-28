package sfvm

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

type RuleFormat struct {
	ruleID                 string
	write                  io.Writer
	editor                 *memedit.MemEditor
	once                   sync.Once
	requireInfoDescKeyType map[SFDescKeyType]bool         // 第一个desc语句中需要的desc item key类型，没有会补全
	infoDescHandler        func(key, value string) string // 第一个desc语句内容的处理函数，可以结合AI补全
}

type RuleFormatOption func(*RuleFormat)

func NewRuleFormat(w io.Writer) *RuleFormat {
	return &RuleFormat{
		ruleID: uuid.NewString(),
		write:  w,
	}
}

func (f *RuleFormat) GetTextFromToken(token CanStartStopToken) string {
	return GetText(f.editor, token)
}

func (f *RuleFormat) Write(format string, args ...any) error {
	if f.write == nil {
		return nil
	}
	s := fmt.Sprintf(format, args...)
	_, err := f.write.Write([]byte(s))
	return err
}

type CanStartStopToken interface {
	GetStop() antlr.Token
	GetStart() antlr.Token
	GetText() string
}

func RuleFormatWithRuleID(ruleID string) RuleFormatOption {
	return func(f *RuleFormat) {
		f.ruleID = ruleID
	}
}

func RuleFormatWithRequireInfoDescKeyType(typ ...SFDescKeyType) RuleFormatOption {
	return func(f *RuleFormat) {
		if f.requireInfoDescKeyType == nil {
			f.requireInfoDescKeyType = make(map[SFDescKeyType]bool)
		}
		for _, t := range typ {
			f.requireInfoDescKeyType[t] = false
		}
	}
}

func RuleFormatWithInfoDescHandler(handler func(key, value string) string) RuleFormatOption {
	return func(f *RuleFormat) {
		f.infoDescHandler = handler
	}
}

func FormatRule(ruleContent string, opts ...RuleFormatOption) (rule string, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = utils.Wrapf(utils.Error(e), "Panic for SyntaxFlow compile")
		}
	}()
	compileErrors := make([]error, 0)
	errHandler := antlr4util.SimpleSyntaxErrorHandler(func(msg string, start, end memedit.PositionIf) {
		compileErrors = append(compileErrors, antlr4util.NewSourceCodeError(msg, start, end))
	})
	errLis := antlr4util.NewErrorListener(func(self *antlr4util.ErrorListener, recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
		antlr4util.StringSyntaxErrorHandler(self, recognizer, offendingSymbol, line, column, msg, e)
		errHandler(self, recognizer, offendingSymbol, line, column, msg, e)
	})

	lexer := sf.NewSyntaxFlowLexer(antlr.NewInputStream(ruleContent))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errLis)
	astParser := sf.NewSyntaxFlowParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	astParser.RemoveErrorListeners()
	astParser.AddErrorListener(errLis)

	flow := astParser.Flow()
	log.Debugf("AST tree:  %v\n", flow.ToStringTree(astParser.RuleNames, astParser))
	if len(errLis.GetErrors()) > 0 {
		return "", utils.Errorf("SyntaxFlow compile error: %v", errLis.GetErrorString())
	}

	editor := memedit.NewMemEditor(ruleContent)
	var buf strings.Builder
	format := NewRuleFormat(&buf)
	format.editor = editor
	for _, opt := range opts {
		opt(format)
	}
	format.Visit(flow, editor)
	rule = buf.String()
	return rule, nil
}

func (f *RuleFormat) Visit(flow sf.IFlowContext, editor *memedit.MemEditor) {
	i, _ := flow.(*sf.FlowContext)
	if i == nil {
		return
	}

	statements, _ := i.Statements().(*sf.StatementsContext)
	if statements == nil {
		return
	}
	// 第一次遍历确定需要补全的desc item
	for _, stmt := range statements.AllStatement() {
		switch stmt := stmt.(type) {
		case *sf.DescriptionContext:
			f.VisitDescriptionFistly(stmt.DescriptionStatement())
		default:
			continue
		}
	}

	// 第二次遍历用于补全
	for i, stmt := range statements.AllStatement() {
		switch stmt := stmt.(type) {
		case *sf.DescriptionContext:
			if i == 0 {
				f.VisitInfoDescription(stmt.DescriptionStatement())
			} else {
				f.Write(f.GetTextFromToken(stmt))
			}
		default:
			f.Write(f.GetTextFromToken(stmt))
		}
	}
}

func (f *RuleFormat) VisitDescriptionFistly(desc sf.IDescriptionStatementContext) {
	i, ok := desc.(*sf.DescriptionStatementContext)
	if i == nil || !ok {
		return
	}
	items, ok := i.DescriptionItems().(*sf.DescriptionItemsContext)
	if !ok || items == nil {
		return
	}

	for _, item := range items.AllDescriptionItem() {
		ret, ok := item.(*sf.DescriptionItemContext)
		if !ok || ret.Comment() != nil {
			continue
		}

		key := mustUnquoteSyntaxFlowString(ret.StringLiteral().GetText())
		switch keyType := ValidDescItemKeyType(strings.ToLower(key)); keyType {
		case SFDescKeyType_Unknown:
			continue
		default:
			if f.requireInfoDescKeyType == nil {
				continue
			}
			_, ok := f.requireInfoDescKeyType[keyType]
			if ok {
				f.requireInfoDescKeyType[keyType] = true
			}
		}
	}
}

func (f *RuleFormat) VisitInfoDescription(desc sf.IDescriptionStatementContext) {
	i, _ := desc.(*sf.DescriptionStatementContext)
	if i == nil {
		return
	}

	f.Write("desc(\n")

	items, ok := i.DescriptionItems().(*sf.DescriptionItemsContext)
	if !ok || items == nil {
		return
	}

	for _, item := range items.AllDescriptionItem() {
		ret, ok := item.(*sf.DescriptionItemContext)
		if !ok || ret.Comment() != nil { // skip comment
			continue
		}
		key := mustUnquoteSyntaxFlowString(ret.StringLiteral().GetText())
		var value string
		if valueItem, ok := ret.DescriptionItemValue().(*sf.DescriptionItemValueContext); ok && valueItem != nil {
			if valueItem.StringLiteral() != nil {
				value = valueItem.GetText()
			}
		}
		if f.infoDescHandler != nil {
			value = f.infoDescHandler(key, value)
		}
		f.Write("\t%s: %s\n", key, value)
	}

	if toAdd := f.getToAddInfoDescKeyType(); toAdd != nil {
		for _, keyType := range toAdd {
			if f.infoDescHandler != nil {
				value := f.infoDescHandler(string(keyType), "")
				f.Write("\t%s: %s\n", string(keyType), value)
			} else {
				f.Write("\t%s\n", string(keyType))
			}
		}
	}
	f.Write(")\n")
}

func (f *RuleFormat) getToAddInfoDescKeyType() []SFDescKeyType {
	if f.requireInfoDescKeyType == nil {
		return []SFDescKeyType{}
	}
	var ret []SFDescKeyType
	for keyType, ok := range f.requireInfoDescKeyType {
		if !ok {
			ret = append(ret, keyType)
		}
	}
	return ret
}

func GetText(editor *memedit.MemEditor, token CanStartStopToken) string {
	if token == nil {
		return ""
	}
	startLine := token.GetStart().GetLine()
	startColumn := token.GetStart().GetColumn()
	endLine, endColumn := GetEndPosition(token.GetStop())
	return editor.GetTextFromPosition(
		editor.GetPositionByLine(startLine, startColumn+1),
		editor.GetPositionByLine(endLine, endColumn+1),
	)
}

func GetEndPosition(t antlr.Token) (int, int) {
	var line, column int
	str := strings.Split(t.GetText(), "\n")
	if len(str) > 1 {
		line = t.GetLine() + len(str) - 1
		column = len(str[len(str)-1])
	} else {
		line = t.GetLine()
		column = t.GetColumn() + len(str[0])
	}
	return line, column
}
