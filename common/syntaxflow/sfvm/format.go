package sfvm

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/utils/yakunquote"

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

	// requireDescKeyType map[SFDescKeyType]bool // 第一个desc语句中需要的desc item key类型，没有会补全
	requireDescKeyType      *omap.OrderedMap[SFDescKeyType, bool] // 第一个desc语句中需要的desc item key类型，没有会补全
	requireAlertDescKeyType *omap.OrderedMap[SFDescKeyType, bool]

	// desc handler
	descHandler  func(key, value string) string
	alertHandler func(name, key, value string) string
}

type RuleFormatOption func(*RuleFormat)

func NewRuleFormat(w io.Writer) *RuleFormat {
	return &RuleFormat{
		ruleID:             uuid.NewString(),
		write:              w,
		requireDescKeyType: omap.NewOrderedMap(map[SFDescKeyType]bool{SFDescKeyType_Rule_Id: false}),
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
	var s string
	if len(args) == 0 {
		s = format
	} else {
		s = fmt.Sprintf(format, args...)
	}
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

// RuleFormatWithRequireInfoDescKeyType 指定info desc必须要有的key，没有的话会使用ai补全
func RuleFormatWithRequireInfoDescKeyType(typ ...SFDescKeyType) RuleFormatOption {
	return func(f *RuleFormat) {
		if f.requireDescKeyType == nil {
			f.requireDescKeyType = omap.NewEmptyOrderedMap[SFDescKeyType, bool]()
		}
		for _, t := range typ {
			f.requireDescKeyType.Set(t, false)
		}
	}
}

// RuleFormatWithRequireAlertDescKeyType 指定alert desc必须要有的key，没有的话会使用ai补全
func RuleFormatWithRequireAlertDescKeyType(typ ...SFDescKeyType) RuleFormatOption {
	return func(f *RuleFormat) {
		if f.requireAlertDescKeyType == nil {
			f.requireAlertDescKeyType = omap.NewEmptyOrderedMap[SFDescKeyType, bool]()
		}
		for _, t := range typ {
			f.requireAlertDescKeyType.Set(t, false)
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
	errHandler := antlr4util.SimpleSyntaxErrorHandler(func(msg string, start, end *memedit.Position) {
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
				f.VisitTestDescription(stmt.DescriptionStatement())
			}
			descCount++
		case *sf.AlertContext:
			f.VisitAlertStatement(stmt.AlertStatement())
		default:
			f.Write(f.GetTextFromToken(stmt))
		}
	}

	if descCount == 0 {
		f.Write("desc(\n")
		f.Write("\trule_id: \"id\"\n")
		f.Write(")\n")
	}

}

func (f *RuleFormat) VisitAlertStatement(alert sf.IAlertStatementContext) {
	alertStmt, ok := alert.(*sf.AlertStatementContext)
	if !ok || alertStmt == nil {
		return
	}
	refVariable, ok := alertStmt.RefVariable().(*sf.RefVariableContext)
	if !ok {
		return
	}

	alertMsg := omap.NewEmptyOrderedMap[string, string]()
	variable := yakunquote.TryUnquote(refVariable.Identifier().GetText())
	if descItem, ok := alertStmt.DescriptionItems().(*sf.DescriptionItemsContext); ok && descItem != nil {
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
			if strings.Contains(value, "MISSING") {
				log.Warnf("desc item %s value is missing, please check the rule: %s", key, f.ruleID)
			}
			descType := ValidDescItemKeyType(key)
			if descType == SFDescKeyType_Unknown {
				alertMsg.Set(key, value)
			} else {
				// 标准化key的名字
				alertMsg.Set(string(descType), value)
			}
		}
	}

	// 检测缺少的desc key
	for _, keyTyp := range f.getAllRequireAlertDescKey() {
		key := string(keyTyp)
		_, b := alertMsg.Get(key)
		if !b {
			alertMsg.Set(key, "")
		}
	}
	f.Write("alert $%s", variable)
	if alertMsg.Len() == 0 {
		f.Write("\n")
		return
	}

	f.Write(" for {\n")
	alertMsg.ForEach(func(key string, value string) bool {
		newVal := f.alertHandler(variable, key, value)
		if lo.Contains([]string{"none", ""}, newVal) {
			return true
		}
		typ := ValidDescItemKeyType(key)

		// 复杂类型使用heredoc
		if IsComplexDescType(typ) {
			upperKey := strings.ToUpper(key)
			res := fmt.Sprintf(`	%s: <<<%s
%s
%s
`, key, upperKey, newVal, upperKey)
			if strings.Contains(res, "MISSING") {
				log.Warnf("alert item %s value is missing, please check the rule: %s", key, res)
			}
			f.Write(res)
		} else {
			res := fmt.Sprintf("\t%s: \"%s\",\n", key, newVal)
			if strings.Contains(res, "MISSING") {
				log.Warnf("alert item %s value is missing, please check the rule: %s", key, res)
			}
			f.Write(res)
		}
		return true
	})
	f.Write("}\n")
}

// VisitInfoDescription 针对第一个desc语句进行处理，主要补全一些规则描述性信息
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
			if f.descHandler == nil || !f.infoDescNeedCompletion(descType) {
				f.Write("\t%s\n", f.GetTextFromToken(item))
				continue
			}

			// 区分StringLiteral、HereDoc和NumberLiteral
			if valueItem.StringLiteral() != nil && !IsComplexDescType(descType) {
				value := f.VisitStringLiteral(valueItem.StringLiteral())
				value = f.descHandler(key, value)
				f.Write("\t%s: \"%s\"\n", key, value)
			} else if valueItem.StringLiteral() != nil && IsComplexDescType(descType) {
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
				if strings.Contains(newValue, "MISSING") {
					log.Warnf("desc item %s value is missing, please check the rule: %s", key, f.ruleID)
				}
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

	// 补充没有的字段（但不包括测试用例，测试用例应该在第二个desc中）
	if toAdd := f.getDescKeyTypesToAdd(); toAdd != nil {
		for _, keyType := range toAdd {
			// 跳过测试用例，测试用例应该在第二个desc中处理
			if IsTestCaseKey(string(keyType)) {
				continue
			}

			if keyType == SFDescKeyType_Rule_Id {
				f.Write("\t%s: \"%s\"\n", string(keyType), f.ruleID)
				continue
			}
			if f.descHandler != nil {
				value := f.descHandler(string(keyType), "")
				// 复杂文本使用heredoc
				if IsComplexDescType(keyType) {
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

// VisitTestDescription 处理除了第一个以外的desc，也就是测试
func (f *RuleFormat) VisitTestDescription(desc sf.IDescriptionStatementContext) {
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
	// 补充测试用例（针对后续desc）
	if testCasesToAdd := f.getTestCasesToAdd(); testCasesToAdd != nil {
		for _, testCaseKey := range testCasesToAdd {
			if f.descHandler != nil {
				value := f.descHandler(testCaseKey, "")
				upperKey := "CODE"
				f.Write("\t\"%s\": <<<%s\n", testCaseKey, upperKey)
				f.Write("%s\n", value)
				f.Write("%s\n", upperKey)
			}
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
	if i.LfHereDoc() != nil {
		return f.VisitLfHereDoc(i.LfHereDoc())
	}
	return f.VisitCrlfHereDoc(i.CrlfHereDoc())
}
func (f *RuleFormat) VisitLfHereDoc(raw sf.ILfHereDocContext) string {
	if raw == nil {
		return ""
	}
	i, ok := raw.(*sf.LfHereDocContext)
	if !ok || i == nil {
		return ""
	}
	if i.LfText() != nil {
		return i.LfText().GetText()
	} else {
		return ""
	}
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

func (f *RuleFormat) infoDescNeedCompletion(descType SFDescKeyType) bool {
	if f == nil || f.requireDescKeyType == nil {
		return false
	}
	_, ok := f.requireDescKeyType.Get(descType)
	return ok
}

func (f *RuleFormat) visitRequireInfoDescKeyType(descType SFDescKeyType) {
	if f == nil || f.requireDescKeyType == nil {
		return
	}

	_, ok := f.requireDescKeyType.Get(descType)
	if ok {
		f.requireDescKeyType.Set(descType, true)
	}
}

func (f *RuleFormat) getDescKeyTypesToAdd() []SFDescKeyType {
	if f.requireDescKeyType == nil {
		return []SFDescKeyType{}
	}
	var ret []SFDescKeyType
	f.requireDescKeyType.ForEach(func(i SFDescKeyType, v bool) bool {
		if !v {
			ret = append(ret, i)
		}
		return true
	})
	return ret
}

func (f *RuleFormat) getAllRequireAlertDescKey() []SFDescKeyType {
	if f.requireAlertDescKeyType == nil {
		return []SFDescKeyType{}
	}
	return lo.MapToSlice(f.requireAlertDescKeyType.GetMap(), func(key SFDescKeyType, value bool) SFDescKeyType {
		return key
	})
}

// IsTestCaseKey 判断是否是测试用例key
func IsTestCaseKey(key string) bool {
	return strings.Contains(key, "://") && (strings.HasPrefix(key, "file://") ||
		strings.HasPrefix(key, "fs://") ||
		strings.HasPrefix(key, "filesystem://") ||
		strings.HasPrefix(key, "safefile://") ||
		strings.HasPrefix(key, "safe-file://") ||
		strings.HasPrefix(key, "negative-file://") ||
		strings.HasPrefix(key, "negativefs://") ||
		strings.HasPrefix(key, "safe-fs://") ||
		strings.HasPrefix(key, "safefilesystem://") ||
		strings.HasPrefix(key, "safe-filesystem://") ||
		strings.HasPrefix(key, "safe://") ||
		strings.HasPrefix(key, "safefs://") ||
		strings.HasPrefix(key, "nfs://"))
}

// needTestCaseCompletion 检查是否需要补全测试用例
func (f *RuleFormat) needTestCaseCompletion(key string) bool {
	if f == nil || f.requireDescKeyType == nil {
		return false
	}
	_, ok := f.requireDescKeyType.Get(SFDescKeyType(key))
	return ok
}

// getTestCasesToAdd 获取需要添加的测试用例
func (f *RuleFormat) getTestCasesToAdd() []string {
	if f.requireDescKeyType == nil {
		return nil
	}
	var ret []string
	f.requireDescKeyType.ForEach(func(i SFDescKeyType, v bool) bool {
		if !v && IsTestCaseKey(string(i)) {
			ret = append(ret, string(i))
		}
		return true
	})
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
