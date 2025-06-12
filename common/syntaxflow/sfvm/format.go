package sfvm

import (
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"io"
	"strconv"
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

	requireDescKeyType map[SFDescKeyType]bool // 第一个desc语句中需要的desc item key类型，没有会补全
	// desc handler
	descHandler  func(key, value string) string
	alertHandler func(name, key, value string) string
}

type RuleFormatOption func(*RuleFormat)

func NewRuleFormat(w io.Writer) *RuleFormat {
	return &RuleFormat{
		ruleID:             uuid.NewString(),
		write:              w,
		requireDescKeyType: map[SFDescKeyType]bool{SFDescKeyType_Rule_Id: false},
		descHandler: func(key, value string) string {
			return value
		},
		alertHandler: func(name, key, value string) string {
			return value
		},
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

func RuleFormatWithRequireDescKeyType(typ ...SFDescKeyType) RuleFormatOption {
	return func(f *RuleFormat) {
		if f.requireDescKeyType == nil {
			f.requireDescKeyType = make(map[SFDescKeyType]bool)
		}
		for _, t := range typ {
			f.requireDescKeyType[t] = false
		}
	}
}

func RuleFormatWithDescHandler(handler func(key, value string) string) RuleFormatOption {
	return func(f *RuleFormat) {
		f.descHandler = handler
	}
}
func RuleFormatWithAlertHandler(h func(name, key, value string) string) RuleFormatOption {
	return func(format *RuleFormat) {
		format.alertHandler = h
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

	descCount := 0
	for _, stmt := range statements.AllStatement() {
		switch stmt := stmt.(type) {
		case *sf.DescriptionContext:
			if descCount == 0 {
				f.VisitInfoDescription(stmt.DescriptionStatement())
			} else {
				f.VisitDescription(stmt.DescriptionStatement())
			}
			descCount++
		case *sf.AlertContext:
			f.VisitAlertStatement(stmt.AlertStatement())
		default:
			f.Write(f.GetTextFromToken(stmt))
		}
	}
}
func (f *RuleFormat) VisitAlertStatement(alert sf.IAlertStatementContext) {
	alertStmt, ok := alert.(*sf.AlertStatementContext)
	if !ok || alertStmt == nil {
		return
	}
	alertMsg := map[string]string{
		"title":    "",
		"title_zh": "",
		"solution": "",
		"desc":     "",
		"level":    "",
	}
	if refVariable, ok := alertStmt.RefVariable().(*sf.RefVariableContext); !ok {
		return
	} else {
		variable := yakunquote.TryUnquote(refVariable.Identifier().GetText())
		defer func() {
			f.Write(fmt.Sprintf("alert $%s", variable))
			isNull := true
			for _, s := range alertMsg {
				if s != "" {
					isNull = false
				}
			}
			if !isNull {
				f.Write(fmt.Sprintf("\tfor {\n"))
				for key, value := range alertMsg {
					newVal := f.alertHandler(variable, key, value)
					if lo.Contains([]string{"none", ""}, newVal) {
						continue
					}
					switch key {
					case "desc", "solution":
						f.Write(fmt.Sprintf(`	%s: <<<CODE
%s
CODE
`, key, newVal))
					default:
						f.Write(fmt.Sprintf("\t%s: \"%s\",\n", key, newVal))
					}
				}
				f.Write("}\n")
			}
		}()
		if alertStmt.DescriptionItems() == nil {
			return
		}
		for _, descItemInterface := range alertStmt.DescriptionItems().(*sf.DescriptionItemsContext).AllDescriptionItem() {
			ret, ok := descItemInterface.(*sf.DescriptionItemContext)
			if !ok || ret.Comment() != nil { // skip comment
				continue
			}
			key := mustUnquoteSyntaxFlowString(ret.StringLiteral().GetText())
			value := ""
			if ret.DescriptionItemValue() == nil {
				continue
			}
			if valueItem, ok := ret.DescriptionItemValue().(*sf.DescriptionItemValueContext); ok && valueItem != nil {
				if valueItem.HereDoc() != nil {
					value = f.VisitHereDoc(valueItem.HereDoc())
				} else if valueItem.StringLiteral() != nil {
					value = mustUnquoteSyntaxFlowString(valueItem.StringLiteral().GetText())
				} else {
					value = valueItem.GetText()
				}
			}
			alertMsg[strings.ToLower(key)] = value
		}
	}
}

// VisitInfoDescription针对第一个desc语句进行处理，主要补全一些规则描述性信息
func (f *RuleFormat) VisitInfoDescription(desc sf.IDescriptionStatementContext) {
	i, _ := desc.(*sf.DescriptionStatementContext)
	if i == nil {
		return
	}

	f.Write("desc(\n")

	items, ok := i.DescriptionItems().(*sf.DescriptionItemsContext)
	if ok && items != nil {
		for _, item := range items.AllDescriptionItem() {
			ret, ok := item.(*sf.DescriptionItemContext)
			if !ok || ret.Comment() != nil { // skip comment
				continue
			}
			key := mustUnquoteSyntaxFlowString(ret.StringLiteral().GetText())
			valueItem, ok := ret.DescriptionItemValue().(*sf.DescriptionItemValueContext)
			if !ok || valueItem == nil {
				continue
			}

			descType := ValidDescItemKeyType(key)
			// 记录访问过的desc item key类型，以便后续确认还缺少哪些key
			f.visitRequireInfoDescKeyType(descType)
			// 不进行字段补全
			if f.descHandler == nil || !f.needCompletion(descType) {
				f.Write("\t%s\n", f.GetTextFromToken(item))
				continue
			}

			// 区分StringLiteral、HereDoc和NumberLiteral
			if valueItem.StringLiteral() != nil && !IsComplexInfoDescType(descType) {
				value := f.VisitStringLiteral(valueItem.StringLiteral())
				value = f.descHandler(key, value)
				f.Write("\t%s: \"%s\"\n", key, value)
			} else if valueItem.StringLiteral() != nil && IsComplexInfoDescType(descType) {
				// 虽然原规则使用StringLiteral写，但是descType是复杂类型，AI补全的可能是复杂文本
				// 所以这里使用heredoc
				value := f.VisitStringLiteral(valueItem.StringLiteral())
				value = f.descHandler(key, value)
				upperKey := strings.ToUpper(key)
				f.Write("\t%s: <<<%s\n", key, upperKey)
				f.Write("%s\n", value)
				f.Write("%s\n", upperKey)
			} else if valueItem.HereDoc() != nil {
				value := f.VisitHereDoc(valueItem.HereDoc())
				newValue := f.descHandler(key, value)
				upperKey := strings.ToUpper(key)
				f.Write("\t%s: <<<%s\n", key, upperKey)
				f.Write("%s\n", newValue)
				f.Write("%s\n", upperKey)
			} else if valueItem.NumberLiteral() != nil {
				value := valueItem.NumberLiteral().GetText()
				newValue := f.descHandler(key, value)
				_, err := strconv.ParseInt(newValue, 10, 64)
				if err == nil {
					f.Write("\t%s: %s\n", key, newValue)
					continue
				}
				_, err = strconv.ParseInt(newValue, 8, 64)
				if err == nil {
					f.Write("\t%s: %s\n", key, newValue)
					continue
				}
				_, err = strconv.ParseInt(newValue, 16, 64)
				if err == nil {
					f.Write("\t%s: %s\n", key, newValue)
					continue
				}
				f.Write("\t%s: \"%s\"\n", key, value)
			}
		}
	}

	// 补充没有的字段
	if toAdd := f.getDescKeyTypesToAdd(); toAdd != nil {
		for _, keyType := range toAdd {
			if keyType == SFDescKeyType_Rule_Id {
				f.Write("\t%s: \"%s\"\n", string(keyType), f.ruleID)
				continue
			}
			if f.descHandler != nil {
				value := f.descHandler(string(keyType), "")
				// 复杂文本使用heredoc
				if IsComplexInfoDescType(keyType) {
					upperKey := strings.ToUpper(string(keyType))
					f.Write("\t%s: <<<%s\n", string(keyType), upperKey)
					f.Write("%s\n", value)
					f.Write("%s\n", upperKey)
					continue
				} else {
					f.Write("\t%s: \"%s\"\n", string(keyType), value)
				}
			} else {
				f.Write("\t%s\n", string(keyType))
			}
		}
	}
	f.Write(")\n")
}

func (f *RuleFormat) VisitDescription(desc sf.IDescriptionStatementContext) {
	i, _ := desc.(*sf.DescriptionStatementContext)
	if i == nil {
		return
	}

	f.Write("desc(\n")
	if items, ok := i.DescriptionItems().(*sf.DescriptionItemsContext); ok && items != nil {
		for _, item := range items.AllDescriptionItem() {
			ret, ok := item.(*sf.DescriptionItemContext)
			if !ok || ret.Comment() != nil { // skip comment
				continue
			}
			f.Write("\t%s\n", f.GetTextFromToken(item))
		}
	}
	f.Write(")\n")
}

func (f *RuleFormat) VisitStringLiteral(raw sf.IStringLiteralContext) string {
	if raw == nil {
		return ""
	}

	i, ok := raw.(*sf.StringLiteralContext)
	if !ok || i == nil {
		return ""
	}
	return mustUnquoteSyntaxFlowString(i.GetText())
}

func (f *RuleFormat) VisitHereDoc(raw sf.IHereDocContext) string {
	if raw == nil {
		return ""
	}

	i, ok := raw.(*sf.HereDocContext)
	if !ok || i == nil {
		return ""
	}
	return f.VisitCrlfHereDoc(i.CrlfHereDoc())
}

func (f *RuleFormat) VisitCrlfHereDoc(raw sf.ICrlfHereDocContext) string {
	if raw == nil {
		return ""
	}
	i, ok := raw.(*sf.CrlfHereDocContext)
	if !ok || i == nil {
		return ""
	}
	if i.CrlfText() != nil {
		return i.CrlfText().GetText()
	} else {
		return ""
	}
}

func (f *RuleFormat) needCompletion(descType SFDescKeyType) bool {
	if f == nil || f.requireDescKeyType == nil {
		return false
	}
	_, ok := f.requireDescKeyType[descType]
	return ok
}

func (f *RuleFormat) visitRequireInfoDescKeyType(descType SFDescKeyType) {
	if f == nil || f.requireDescKeyType == nil {
		return
	}

	_, ok := f.requireDescKeyType[descType]
	if ok {
		f.requireDescKeyType[descType] = true
	}
}

func (f *RuleFormat) getDescKeyTypesToAdd() []SFDescKeyType {
	if f.requireDescKeyType == nil {
		return []SFDescKeyType{}
	}
	var ret []SFDescKeyType
	for keyType, ok := range f.requireDescKeyType {
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
