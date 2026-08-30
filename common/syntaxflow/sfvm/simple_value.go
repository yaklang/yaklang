package sfvm

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/regexp-utils"
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

	// files backs chained context search ($a.regexp(...)): the path→content
	// map the hit was produced from. Set by sfpattern when creating hits.
	files map[string]string
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

// SetFiles attaches the source path→content map so chained FileFilter calls
// ($a.regexp(...)) can search within the hit's file. Nil-safe.
func (v *SimpleValue) SetFiles(files map[string]string) {
	if v != nil {
		v.files = files
	}
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

// FileFilter on a simple hit runs a chained context search: the regex is
// applied to the file the hit came from (default scope=file), or to the hit's
// own range (scope=region). This powers `$a.regexp(...)` — searching around
// prior hits. Only regexp match type is supported.
func (v *SimpleValue) FileFilter(name, matchType string, paramMap map[string]string, patterns []string) (Values, error) {
	if v == nil {
		return nil, utils.Error("simple value: nil hit")
	}
	switch strings.ToLower(matchType) {
	case "regexp", "re", "pattern_regex", "pattern-regex":
	default:
		return nil, utils.Errorf("simple value: unsupported chained match type %q (use regexp)", matchType)
	}
	if v.path == "" {
		return nil, utils.Error("simple value: hit has no file path")
	}
	if v.files == nil {
		return nil, utils.Error("simple value: no files backing for chained search")
	}
	content, ok := v.files[v.path]
	if !ok {
		return nil, utils.Errorf("simple value: file %q not in backing files", v.path)
	}

	// scope=region searches within the hit's own range; default scope=file
	// searches the whole file the hit lives in.
	searchFrom, searchTo := 0, len(content)
	if paramMap != nil && strings.EqualFold(paramMap["scope"], "region") {
		searchFrom, searchTo = v.start, v.end
		if searchFrom < 0 || searchTo > len(content) || searchTo < searchFrom {
			return nil, utils.Errorf("simple value: hit range [%d,%d) out of file bounds", v.start, v.end)
		}
	}

	var cleaned []string
	for _, s := range patterns {
		if strings.TrimSpace(s) != "" {
			cleaned = append(cleaned, s)
		}
	}
	if len(cleaned) == 0 {
		return nil, utils.Error("simple value: no content patterns")
	}

	// pattern_regex_not / pattern_not_regex in the CHAINED form: the hit itself
	// is the positive; every param is a negative. Keep the hit only when no
	// negative match overlaps it (Semgrep pattern-not-regex).
	if paramMap != nil && (paramMap["__sf_pattern_not_list"] == "1" || paramMap["__sf_pattern_not"] == "1") {
		if len(cleaned) == 0 {
			return nil, utils.Error("simple value: negative filter requires at least one pattern")
		}
		for _, expr := range cleaned {
			yak := regexp_utils.NewYakRegexpUtils(expr)
			indexs, err := yak.FindAllSubmatchIndex(content[searchFrom:searchTo])
			if err != nil {
				log.Warnf("simple value negative regexp match error: %s", err)
				continue
			}
			for _, index := range indexs {
				if len(index) < 2 {
					continue
				}
				negStart, negEnd := searchFrom+index[0], searchFrom+index[1]
				if v.start < negEnd && negStart < v.end {
					return NewEmptyValues(), nil // dropped
				}
			}
		}
		return ValuesOf(v), nil
	}

	var out []ValueOperator
	for _, expr := range cleaned {
		yak := regexp_utils.NewYakRegexpUtils(expr)
		indexs, err := yak.FindAllSubmatchIndex(content[searchFrom:searchTo])
		if err != nil {
			log.Warnf("simple value chained regexp match error: %s", err)
			continue
		}
		for _, index := range indexs {
			if len(index) < 2 {
				continue
			}
			start, end := searchFrom+index[0], searchFrom+index[1]
			if start < 0 || end > len(content) || end < start {
				continue
			}
			hit := NewSimpleValue(content[start:end], v.path, start, end)
			hit.SetFiles(v.files)
			out = append(out, hit)
		}
	}
	return NewValues(out), nil
}

// chainedRegexpWithNegatives mirrors sfpattern.MatchRegexpWithNegatives for a
// single hit's context: positive hits minus hits overlapping any negative match.
func chainedRegexpWithNegatives(v *SimpleValue, content string, from, to int, positive string, negatives []string) (Values, error) {
	yak := regexp_utils.NewYakRegexpUtils(positive)
	indexs, err := yak.FindAllSubmatchIndex(content[from:to])
	if err != nil {
		return nil, err
	}
	type posHit struct {
		start, end int
	}
	var pos []posHit
	for _, index := range indexs {
		if len(index) >= 2 {
			pos = append(pos, posHit{start: from + index[0], end: from + index[1]})
		}
	}
	if len(pos) == 0 {
		return NewEmptyValues(), nil
	}
	var negRanges []posHit
	for _, expr := range negatives {
		yak := regexp_utils.NewYakRegexpUtils(expr)
		indexs, err := yak.FindAllSubmatchIndex(content[from:to])
		if err != nil {
			log.Warnf("simple value negative regexp match error: %s", err)
			continue
		}
		for _, index := range indexs {
			if len(index) >= 2 {
				negRanges = append(negRanges, posHit{start: from + index[0], end: from + index[1]})
			}
		}
	}
	var out []ValueOperator
	for _, h := range pos {
		overlap := false
		for _, nr := range negRanges {
			if h.start < nr.end && nr.start < h.end {
				overlap = true
				break
			}
		}
		if overlap {
			continue
		}
		hit := NewSimpleValue(content[h.start:h.end], v.path, h.start, h.end)
		hit.SetFiles(v.files)
		out = append(out, hit)
	}
	return NewValues(out), nil
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
	files           map[string]string
	matcher         FileFilterFunc
	programName     string
	sourceMu        sync.RWMutex
	sourceHitOffset int
	sourceHitLimit  int
	sourceHitTotal  int
	sourceHitKey    string
	sourceHitCache  []SourceHit
	sourceEditors   map[string]*memedit.MemEditor
}

// SourceHit is a raw source match before it is wrapped as a ValueOperator.
type SourceHit struct {
	Path  string
	Text  string
	Start int
	End   int
}

// FileFilterFunc is injected by sfpattern to avoid sfvm→sfpattern import cycles
// when wiring is done from the opcode side (sfvm calls sfpattern package).
type FileFilterFunc func(root *PatternRoot, files map[string]string, pathPattern string, matchType string, paramMap map[string]string, patterns []string) (Values, error)

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

// SetSourceHitBatch selects one bounded raw-hit window. Cache and editor state
// are intentionally preserved between windows of the same query.
func (r *PatternRoot) SetSourceHitBatch(offset, limit int) {
	if r == nil {
		return
	}
	r.sourceMu.Lock()
	defer r.sourceMu.Unlock()
	r.sourceHitOffset = max(0, offset)
	r.sourceHitLimit = max(0, limit)
}

// SourceHitBatch returns the current raw-hit window and its total.
func (r *PatternRoot) SourceHitBatch() (offset int, limit int, total int) {
	if r == nil {
		return 0, 0, 0
	}
	r.sourceMu.RLock()
	defer r.sourceMu.RUnlock()
	return r.sourceHitOffset, r.sourceHitLimit, r.sourceHitTotal
}

// SetSourceHits caches raw hits for repeated bounded executions of one filter.
func (r *PatternRoot) SetSourceHits(key string, hits []SourceHit) {
	if r == nil {
		return
	}
	r.sourceMu.Lock()
	defer r.sourceMu.Unlock()
	if key != r.sourceHitKey {
		r.sourceHitKey = key
		r.sourceHitCache = append([]SourceHit(nil), hits...)
		r.sourceEditors = make(map[string]*memedit.MemEditor)
	}
	r.sourceHitTotal = len(r.sourceHitCache)
}

// SourceHits returns cached raw hits when the filter key matches.
func (r *PatternRoot) SourceHits(key string) ([]SourceHit, bool) {
	if r == nil {
		return nil, false
	}
	r.sourceMu.RLock()
	defer r.sourceMu.RUnlock()
	if key != r.sourceHitKey {
		return nil, false
	}
	return r.sourceHitCache, true
}

// SourceHitEditor returns one editor per file across all batches. Reusing the
// editor prevents each batch from retaining duplicate line/rune maps.
func (r *PatternRoot) SourceHitEditor(path string) *memedit.MemEditor {
	if r == nil {
		return nil
	}
	r.sourceMu.Lock()
	defer r.sourceMu.Unlock()
	if r.sourceEditors == nil {
		r.sourceEditors = make(map[string]*memedit.MemEditor)
	}
	if editor, ok := r.sourceEditors[path]; ok {
		return editor
	}
	content, ok := r.files[path]
	if !ok {
		return nil
	}
	editor := memedit.NewMemEditorWithFileUrl(content, path)
	r.sourceEditors[path] = editor
	return editor
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
	vals, err := r.matcher(r, r.files, pathPattern, matchType, paramMap, patterns)
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
