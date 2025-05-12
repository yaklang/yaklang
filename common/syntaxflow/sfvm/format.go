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
	ruleID string
	write  io.Writer
	editor *memedit.MemEditor
	once   sync.Once
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
	for _, stmt := range statements.AllStatement() {
		switch stmt := stmt.(type) {
		case *sf.DescriptionContext:
			f.VisitDescription(stmt.DescriptionStatement())
		default:
			log.Debugf("statement: %s", stmt.GetText())
			f.Write(f.GetTextFromToken(stmt))
		}
	}
}

func (f *RuleFormat) VisitDescription(desc sf.IDescriptionStatementContext) {
	log.Debugf("description: %s", desc.GetText())
	i, _ := desc.(*sf.DescriptionStatementContext)
	if i == nil {
		return
	}

	f.Write("desc(\n")
	resultId := ""
	if items, ok := i.DescriptionItems().(*sf.DescriptionItemsContext); ok && items != nil {
		for _, item := range items.AllDescriptionItem() {
			ret, ok := item.(*sf.DescriptionItemContext)
			if !ok || ret.Comment() != nil { // skip comment
				continue
			}
			key := mustUnquoteSyntaxFlowString(ret.StringLiteral().GetText())
			switch keyLower := strings.ToLower(key); keyLower {
			case "rule_id", "id":
				var value string
				if valueItem, ok := ret.DescriptionItemValue().(*sf.DescriptionItemValueContext); ok && valueItem != nil {
					if valueItem.StringLiteral() != nil {
						value = mustUnquoteSyntaxFlowString(valueItem.StringLiteral().GetText())
					} else {
						value = valueItem.GetText()
					}
				}
				resultId = value
				if value == "" {
					log.Errorf("set result-id but is empty: %s", desc.GetText())
				}
			default:
				// f.WriteToken(item)
				f.Write("\t%s\n", f.GetTextFromToken(item))
			}
		}
	}

	if resultId == "" {
		resultId = f.ruleID
	}

	f.once.Do(func() {
		f.Write("\trule_id: \"%v\"\n", resultId)
	})
	f.Write(")\n")
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
