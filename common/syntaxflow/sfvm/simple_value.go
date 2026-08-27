package sfvm

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// SimpleValue is a minimal ValueOperator for non-SSA pattern hits (sfpattern).
// It only carries matched text / path / offsets — no SSA Program dependency.
// When a full-file editor is attached (sfpattern regexp hits), the hit is
// anchored inside the whole file so risks get real line numbers / context.
type SimpleValue struct {
	text   string
	path   string
	start  int // rune or byte start (display)
	end    int
	empty  bool
	editor *memedit.MemEditor // optional full-file editor containing this hit
}

// NewSimpleValue builds a pattern hit value.
func NewSimpleValue(text string, path string, start, end int) *SimpleValue {
	return &SimpleValue{text: text, path: path, start: start, end: end}
}

// NewSimpleValueWithEditor builds a pattern hit value anchored inside a
// full-file editor. start/end are byte offsets into that editor's content.
func NewSimpleValueWithEditor(text string, path string, start, end int, editor *memedit.MemEditor) *SimpleValue {
	return &SimpleValue{text: text, path: path, start: start, end: end, editor: editor}
}

// FileEditor returns the attached full-file editor (nil when absent).
func (v *SimpleValue) FileEditor() *memedit.MemEditor {
	if v == nil {
		return nil
	}
	return v.editor
}

// NewSimpleConst builds a const-like simple value (no path).
func NewSimpleConst(text string) *SimpleValue {
	return &SimpleValue{text: fmt.Sprint(text)}
}

func (v *SimpleValue) String() string {
	if v == nil {
		return ""
	}
	return v.text
}

// GetId returns a stable positive identity for a pattern hit so value-set
// operations (merge/intersect/remove) can dedup by path+range+text.
func (v *SimpleValue) GetId() int64 {
	if v == nil {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(v.path))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(v.text))
	_, _ = h.Write([]byte{0})
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[:8], uint64(v.start))
	binary.LittleEndian.PutUint64(buf[8:], uint64(v.end))
	_, _ = h.Write(buf[:])
	id := int64(h.Sum64() & 0x7fffffffffffffff)
	if id == 0 {
		id = 1
	}
	return id
}

func (v *SimpleValue) Path() string {
	if v == nil {
		return ""
	}
	return v.path
}

func (v *SimpleValue) Start() int {
	if v == nil {
		return 0
	}
	return v.start
}

func (v *SimpleValue) End() int {
	if v == nil {
		return 0
	}
	return v.end
}

func (v *SimpleValue) IsMap() bool  { return false }
func (v *SimpleValue) IsList() bool { return false }
func (v *SimpleValue) IsEmpty() bool {
	return v == nil || v.empty
}
func (v *SimpleValue) ShouldUseConditionCandidate() bool { return false }
func (v *SimpleValue) GetOpcode() string                 { return "const" }
func (v *SimpleValue) GetBinaryOperator() string         { return "" }
func (v *SimpleValue) GetUnaryOperator() string          { return "" }

func (v *SimpleValue) ExactMatch(context.Context, ssadb.MatchMode, string) (bool, Values, error) {
	return false, nil, nil
}
func (v *SimpleValue) GlobMatch(context.Context, ssadb.MatchMode, string) (bool, Values, error) {
	return false, nil, nil
}
func (v *SimpleValue) RegexpMatch(context.Context, ssadb.MatchMode, string) (bool, Values, error) {
	return false, nil, nil
}
func (v *SimpleValue) GetCalled() (Values, error)                    { return nil, nil }
func (v *SimpleValue) GetCallActualParams(int, bool) (Values, error) { return nil, nil }
func (v *SimpleValue) GetFields() (Values, error)                    { return nil, nil }
func (v *SimpleValue) GetSyntaxFlowUse() (Values, error)             { return nil, nil }
func (v *SimpleValue) GetSyntaxFlowDef() (Values, error)             { return nil, nil }
func (v *SimpleValue) GetSyntaxFlowTopDef(*SFFrameResult, *Config, ...*RecursiveConfigItem) (Values, error) {
	return nil, nil
}
func (v *SimpleValue) GetSyntaxFlowBottomUse(*SFFrameResult, *Config, ...*RecursiveConfigItem) (Values, error) {
	return nil, nil
}
func (v *SimpleValue) ListIndex(int) (ValueOperator, error) {
	return nil, utils.Error("simple value: list index unsupported")
}
func (v *SimpleValue) AppendPredecessor(ValueOperator, ...AnalysisContextOption) error {
	return nil
}

// FileFilter on a simple hit is unsupported; pattern roots implement PatternRoot.
func (v *SimpleValue) FileFilter(string, string, map[string]string, []string) (Values, error) {
	return nil, utils.Error("simple value: FileFilter unsupported")
}

func (v *SimpleValue) CompareString(items *StringComparator) (Values, []bool) {
	if v == nil || items == nil {
		return nil, []bool{false}
	}
	names := []string{v.text, yakunquote.TryUnquote(v.text)}
	if v.path != "" {
		names = append(names, v.path)
	}
	ok := items.Matches(names...)
	return ValuesOf(v), []bool{ok}
}

func (v *SimpleValue) CompareOpcode(*OpcodeComparator) (Values, []bool) {
	return ValuesOf(v), []bool{false}
}

func (v *SimpleValue) CompareConst(comparator *ConstComparator) bool {
	if v == nil || comparator == nil {
		return false
	}
	return comparator.Matches(v.text)
}

func (v *SimpleValue) NewConst(i any, _ ...*memedit.Range) ValueOperator {
	return NewSimpleConst(fmt.Sprint(i))
}

func (v *SimpleValue) GetAnchorBitVector() *utils.BitVector { return nil }
func (v *SimpleValue) SetAnchorBitVector(*utils.BitVector)  {}

// PatternRoot is the feed root for source-mode scans: holds files and responds to
// FileFilter by delegating to a registered regexp matcher (set by sfpattern).
type PatternRoot struct {
	files       map[string]string
	matcher     FileFilterFunc
	programName string
}

// FileFilterFunc is injected by sfpattern to avoid sfvm→sfpattern import cycles
// when wiring is done from the opcode side (sfvm calls sfpattern package).
type FileFilterFunc func(files map[string]string, pathPattern string, matchType string, paramMap map[string]string, patterns []string) (Values, error)

// NewPatternRoot builds a source-scan root from path→content.
func NewPatternRoot(files map[string]string) *PatternRoot {
	if files == nil {
		files = map[string]string{}
	}
	return &PatternRoot{files: files}
}

// SetFileFilterMatcher registers the regexp/xpath/json implementation (usually sfpattern).
func (r *PatternRoot) SetFileFilterMatcher(fn FileFilterFunc) {
	if r != nil {
		r.matcher = fn
	}
}

// SetProgramName records the owning program name so hit editors produce
// program-scoped URLs and ir_source hashes (aligned with compiled programs).
func (r *PatternRoot) SetProgramName(name string) {
	if r != nil {
		r.programName = name
	}
}

// GetProgramName returns the recorded program name (may be empty).
func (r *PatternRoot) GetProgramName() string {
	if r == nil {
		return ""
	}
	return r.programName
}

// Files returns the underlying path→content map.
func (r *PatternRoot) Files() map[string]string {
	if r == nil {
		return nil
	}
	return r.files
}

func (r *PatternRoot) String() string { return "pattern-root" }
func (r *PatternRoot) IsMap() bool    { return false }
func (r *PatternRoot) IsList() bool   { return false }
func (r *PatternRoot) IsEmpty() bool  { return r == nil || len(r.files) == 0 }
func (r *PatternRoot) ShouldUseConditionCandidate() bool {
	return false
}
func (r *PatternRoot) GetOpcode() string         { return "" }
func (r *PatternRoot) GetBinaryOperator() string { return "" }
func (r *PatternRoot) GetUnaryOperator() string  { return "" }
func (r *PatternRoot) ExactMatch(context.Context, ssadb.MatchMode, string) (bool, Values, error) {
	return false, nil, nil
}
func (r *PatternRoot) GlobMatch(context.Context, ssadb.MatchMode, string) (bool, Values, error) {
	return false, nil, nil
}
func (r *PatternRoot) RegexpMatch(context.Context, ssadb.MatchMode, string) (bool, Values, error) {
	return false, nil, nil
}
func (r *PatternRoot) GetCalled() (Values, error)                    { return nil, nil }
func (r *PatternRoot) GetCallActualParams(int, bool) (Values, error) { return nil, nil }
func (r *PatternRoot) GetFields() (Values, error)                    { return nil, nil }
func (r *PatternRoot) GetSyntaxFlowUse() (Values, error)             { return nil, nil }
func (r *PatternRoot) GetSyntaxFlowDef() (Values, error)             { return nil, nil }
func (r *PatternRoot) GetSyntaxFlowTopDef(*SFFrameResult, *Config, ...*RecursiveConfigItem) (Values, error) {
	return nil, nil
}
func (r *PatternRoot) GetSyntaxFlowBottomUse(*SFFrameResult, *Config, ...*RecursiveConfigItem) (Values, error) {
	return nil, nil
}
func (r *PatternRoot) ListIndex(int) (ValueOperator, error) {
	return nil, utils.Error("pattern root: list index unsupported")
}
func (r *PatternRoot) AppendPredecessor(ValueOperator, ...AnalysisContextOption) error {
	return nil
}

func (r *PatternRoot) FileFilter(pathPattern, matchType string, paramMap map[string]string, patterns []string) (Values, error) {
	if r == nil {
		return nil, utils.Error("nil pattern root")
	}
	if r.matcher == nil {
		return nil, utils.Error("pattern root: file filter matcher not registered")
	}
	vals, err := r.matcher(r.files, pathPattern, matchType, paramMap, patterns)
	if err != nil {
		return nil, err
	}
	// Propagate the program name into hit editors so file URLs / ir-source
	// hashes match the convention of compiled programs (files get deduped
	// against compile artifacts downstream).
	if r.programName != "" {
		for _, v := range vals {
			if sv, ok := v.(*SimpleValue); ok && sv != nil && sv.editor != nil {
				sv.editor.SetProgramName(r.programName)
			}
		}
	}
	return vals, nil
}

func (r *PatternRoot) CompareString(*StringComparator) (Values, []bool) {
	return nil, []bool{false}
}
func (r *PatternRoot) CompareOpcode(*OpcodeComparator) (Values, []bool) {
	return nil, []bool{false}
}
func (r *PatternRoot) CompareConst(*ConstComparator) bool { return false }
func (r *PatternRoot) NewConst(i any, _ ...*memedit.Range) ValueOperator {
	return NewSimpleConst(fmt.Sprint(i))
}
func (r *PatternRoot) GetAnchorBitVector() *utils.BitVector { return nil }
func (r *PatternRoot) SetAnchorBitVector(*utils.BitVector)  {}
