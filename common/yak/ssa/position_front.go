package ssa

import (
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

func (b *FunctionBuilder) AppendBlockRange() {
	blockRange := b.CurrentBlock.GetRange()
	if blockRange == nil {
		blockRange = b.CurrentRange
	} else {
		blockRange.Add(b.CurrentRange)
	}
	b.CurrentBlock.SetRange(blockRange)
}

func (b *FunctionBuilder) SetRangeFromTerminalNode(node antlr.TerminalNode) func() {
	return b.SetRange(NewToken(node))
}
func (b *FunctionBuilder) SetRange(token CanStartStopToken) func() {
	r := GetRange(b.GetEditor(), token)
	if r == nil {
		return func() {}
	}
	backup := b.CurrentRange
	b.CurrentRange = r

	return func() {
		b.CurrentRange = backup
	}
}

func (b *FunctionBuilder) SetRangeWithCommonTokenLoc(loc CommonTokenLoc) func() {
	r := GetRangeWithCommonTokenLoc(b.GetEditor(), loc)
	if r == nil {
		return func() {}
	}
	backup := b.CurrentRange
	b.CurrentRange = r

	return func() {
		b.CurrentRange = backup
	}
}

func (b *FunctionBuilder) GetRangesByText(searchText string) []*memedit.Range {
	return GetRangesByText(b.GetEditor(), searchText)
}

func (b *FunctionBuilder) SetRangeByRangeIf(rng *memedit.Range) {
	if b == nil {
		return
	}
	b.CurrentRange = rng
}

func (b *FunctionBuilder) SetEmptyRange() {
	editor := b.GetEditor()
	if editor == nil {
		editor = memedit.NewMemEditor("")
	}
	r := editor.GetFullRange()
	b.CurrentRange = r
}

// func (b *FunctionBuilder) SetRangeInit(p *memedit.MemEditor) {
// 	if b.CurrentRange != nil {
// 		log.Warnf("init for set-range for function builder: %v, but the current range is not nil", b.name)
// 		return
// 	}

// 	fullRange := p.GetFullRange()
// 	if fullRange == nil {
// 		log.Warnf("init for set-range for function builder: %v, but the full range is nil", b.name)
// 		return
// 	}
// 	b.CurrentRange = NewRange(p, fullRange.GetStart(), fullRange.GetEnd())
// }

func (b *FunctionBuilder) GetCurrentRange(fallback CanStartStopToken) *memedit.Range {
	if b.CurrentRange != nil {
		return b.CurrentRange
	}
	if fallback != nil {
		log.Warn("use fallback for GetCurrentRange, unhealthy operation")
		return GetRange(b.GetEditor(), fallback)
	}
	log.Error("fallback for GetCurrentRange is nil..., use (1:1, 1000,1) fallback, bad operation")
	return b.GetEditor().GetRangeOffset(1, 1000)
	// return NewRange(b.GetEditor(), NewPosition(1, 1), NewPosition(1000, 1))
}

func (b *FunctionBuilder) GetRangeByToken(r CanStartStopToken) *memedit.Range {
	return GetRange(b.GetEditor(), r)
}

// / ============================== Token ==============================
type CanStartStopToken interface {
	GetStop() antlr.Token
	GetStart() antlr.Token
	GetText() string
}

type CommonTokenLoc struct {
	text      string
	startLine int
	startCol  int
	endLine   int
	endCol    int
}

func NewCommonTokenLoc(text string, startLine int, startCol int, endLine int, endCol int) CommonTokenLoc {
	return CommonTokenLoc{
		text:      text,
		startLine: startLine,
		startCol:  startCol,
		endLine:   endLine,
		endCol:    endCol,
	}
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

func GetRange(editor *memedit.MemEditor, token CanStartStopToken) *memedit.Range {
	startToken := token.GetStart()
	endToken := token.GetStop()
	if startToken == nil || endToken == nil {
		return nil
	}

	endLine, endColumn := GetEndPosition(endToken)
	return editor.GetRangeByPosition(
		editor.GetPositionByLine(startToken.GetLine(), startToken.GetColumn()+1),
		editor.GetPositionByLine(endLine, endColumn+1),
	)
}

func GetRangeWithCommonTokenLoc(editor *memedit.MemEditor, loc CommonTokenLoc) *memedit.Range {
	return editor.GetRangeByPosition(
		editor.GetPositionByLine(loc.startLine+1, loc.startCol+1),
		editor.GetPositionByLine(loc.endLine+1, loc.endCol+1),
	)
}

func GetRangesByText(editor *memedit.MemEditor, searchText string) []*memedit.Range {
	if editor == nil || searchText == "" {
		return nil
	}
	var ranges []*memedit.Range
	editor.FindStringRange(searchText, func(rangeIf *memedit.Range) error {
		ranges = append(ranges, rangeIf)
		return nil
	})
	return ranges
}

func GetFirstRangeByText(editor *memedit.MemEditor, searchText string) *memedit.Range {
	if editor == nil {
		return nil
	}
	indices := strings.Index(editor.GetSourceCode(), searchText)
	return editor.GetRangeByPosition(editor.GetPositionByOffset(indices), editor.GetPositionByOffset(indices+len(searchText)))
}

type Token struct {
	start antlr.Token
	end   antlr.Token
	text  string
}

func NewToken(node antlr.TerminalNode) *Token {
	return &Token{
		start: node.GetSymbol(),
		end:   node.GetSymbol(),
		text:  node.GetText(),
	}
}

func (t *Token) GetStart() antlr.Token {
	return t.start
}
func (t *Token) GetStop() antlr.Token {
	return t.end
}
func (t *Token) GetText() string {
	return t.text
}

var _ CanStartStopToken = (*Token)(nil)
