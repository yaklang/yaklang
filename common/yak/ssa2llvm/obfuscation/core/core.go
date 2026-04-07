package core

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type Stage int

const (
	StageSSAPre Stage = iota
	StageSSAPost
	StageLLVM
)

type Kind int

const (
	KindSSA Kind = iota
	KindHybrid
	KindLLVM
)

func (k Kind) String() string {
	switch k {
	case KindSSA:
		return "ssa"
	case KindHybrid:
		return "hybrid"
	case KindLLVM:
		return "llvm"
	default:
		return "unknown"
	}
}

type Context struct {
	Stage Stage

	SSA  *ssa.Program
	LLVM llvm.Module

	// EntryFunction is the user-requested entry function name for the build.
	// Hybrid obfuscators may use it to decide how to transform whole-program control flow.
	EntryFunction string

	// InstrTags carries obfuscator-owned instruction markers across SSA and LLVM
	// stages without extending the SSA IR schema itself.
	InstrTags map[int64]string

	// Selections maps obfuscator name → set of function names resolved from the
	// obf policy. When nil (no policy), obfuscators use their default behaviour
	// (typically all functions). When non-nil, an obfuscator should only operate
	// on the functions listed for its name; an absent key means "no functions".
	Selections map[string]map[string]struct{}

	// ObfData is a generic cross-stage data bag for obfuscators that need to
	// pass state between SSAPre/SSAPost/LLVM stages. Each obfuscator should
	// store data under its own name key.
	ObfData map[string]any

	// BodyReplacedFuncs tracks functions whose bodies have been claimed by a
	// body-replace obfuscator (e.g. virtualize).  Map key is the function name,
	// value is the obfuscator that owns it.  Other obfuscators (e.g. callret)
	// must treat these functions as opaque and not attempt to inline or
	// flatten them.
	BodyReplacedFuncs map[string]string

	// BuildSeed is an optional build-level seed for diversification.
	// When non-nil, obfuscators may use it to vary their output per build.
	// Populated from the profile's SeedPolicy.
	BuildSeed []byte
}

// IsSelected returns true if funcName is selected for the given obfuscator.
// When Selections is nil (no policy), always returns true (backward compat).
// When Selections is set but the obfuscator has no entry, returns false.
func (ctx *Context) IsSelected(obfName, funcName string) bool {
	if ctx == nil || ctx.Selections == nil {
		return true
	}
	funcs, ok := ctx.Selections[obfName]
	if !ok {
		return false
	}
	_, selected := funcs[funcName]
	return selected
}

// HasSelections returns true if policy-based selections are active.
func (ctx *Context) HasSelections() bool {
	return ctx != nil && ctx.Selections != nil
}

// SetObfData stores obfuscator-specific cross-stage data under the given key.
func (ctx *Context) SetObfData(key string, value any) {
	if ctx == nil {
		return
	}
	if ctx.ObfData == nil {
		ctx.ObfData = make(map[string]any)
	}
	ctx.ObfData[key] = value
}

// GetObfData retrieves obfuscator-specific cross-stage data.
func (ctx *Context) GetObfData(key string) (any, bool) {
	if ctx == nil || ctx.ObfData == nil {
		return nil, false
	}
	v, ok := ctx.ObfData[key]
	return v, ok
}

// MarkBodyReplaced records that obfName has claimed funcName via body replacement.
func (ctx *Context) MarkBodyReplaced(obfName, funcName string) {
	if ctx == nil {
		return
	}
	if ctx.BodyReplacedFuncs == nil {
		ctx.BodyReplacedFuncs = make(map[string]string)
	}
	ctx.BodyReplacedFuncs[funcName] = obfName
}

// IsBodyReplaced returns true if funcName has been claimed by a body-replace
// obfuscator. Other obfuscators should treat such functions as opaque.
func (ctx *Context) IsBodyReplaced(funcName string) bool {
	if ctx == nil || ctx.BodyReplacedFuncs == nil {
		return false
	}
	_, ok := ctx.BodyReplacedFuncs[funcName]
	return ok
}

type Obfuscator interface {
	Name() string
	Kind() Kind
	Apply(*Context) error
}

type Info struct {
	Name string
	Kind Kind
}

var Default = make(map[string]Obfuscator)

func Register(obfuscator Obfuscator) {
	if obfuscator == nil {
		log.Warnf("skip nil obfuscator registration")
		return
	}

	name := normalizeName(obfuscator.Name())
	if name == "" {
		log.Warnf("skip obfuscator registration with empty name")
		return
	}
	if _, exists := Default[name]; exists {
		log.Warnf("skip duplicate obfuscator registration %q", name)
		return
	}

	Default[name] = obfuscator
}

func Apply(ctx *Context, names []string) error {
	if ctx == nil {
		return nil
	}
	return applyStage(ctx, names)
}

func List() []Info {
	names := sortedKeys(Default)
	out := make([]Info, 0, len(names))
	for _, name := range names {
		obf := Default[name]
		kind := KindSSA
		if obf != nil {
			kind = obf.Kind()
		}
		out = append(out, Info{Name: name, Kind: kind})
	}
	return out
}

func ListByKind(kind Kind) []string {
	names := sortedKeys(Default)
	out := make([]string, 0, len(names))
	for _, name := range names {
		obf := Default[name]
		if obf == nil || obf.Kind() != kind {
			continue
		}
		out = append(out, name)
	}
	return out
}

func applyStage(ctx *Context, names []string) error {
	resolved, err := expandNames("obf", names, sortedKeys(Default))
	if err != nil {
		return err
	}

	if ctx.Stage == StageSSAPre {
		// SSA-only first, then hybrid pre-SSA.
		if err := applyKinds(ctx, resolved, KindSSA); err != nil {
			return err
		}
		return applyKinds(ctx, resolved, KindHybrid)
	}

	if ctx.Stage == StageSSAPost {
		// Post-SSA runs only hybrid obfuscators on the lowered SSA form.
		return applyKinds(ctx, resolved, KindHybrid)
	}

	// LLVM: hybrid first, then LLVM-only.
	if err := applyKinds(ctx, resolved, KindHybrid); err != nil {
		return err
	}
	return applyKinds(ctx, resolved, KindLLVM)
}

func applyKinds(ctx *Context, resolved []string, kind Kind) error {
	for _, name := range resolved {
		obf := Default[name]
		if obf == nil || obf.Kind() != kind {
			continue
		}
		if err := obf.Apply(ctx); err != nil {
			return fmt.Errorf("%s obfuscator %q failed: %w", ctx.StageLabel(), name, err)
		}
	}
	return nil
}

func (ctx *Context) StageLabel() string {
	if ctx == nil {
		return "obf"
	}
	switch ctx.Stage {
	case StageSSAPre:
		return "ssa-pre"
	case StageSSAPost:
		return "ssa-post"
	case StageLLVM:
		return "llvm"
	default:
		return "obf"
	}
}

func (ctx *Context) SetInstructionTag(id int64, tag string) {
	if ctx == nil || id <= 0 {
		return
	}
	if tag == "" {
		if ctx.InstrTags != nil {
			delete(ctx.InstrTags, id)
		}
		return
	}
	if ctx.InstrTags == nil {
		ctx.InstrTags = make(map[int64]string)
	}
	ctx.InstrTags[id] = tag
}

func (ctx *Context) InstructionTag(id int64) string {
	if ctx == nil || id <= 0 || ctx.InstrTags == nil {
		return ""
	}
	return ctx.InstrTags[id]
}

func NormalizeNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		for _, part := range strings.Split(name, ",") {
			normalized := normalizeName(part)
			if normalized == "" {
				continue
			}
			out = append(out, normalized)
		}
	}
	return out
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func expandNames(stage string, patterns []string, available []string) ([]string, error) {
	normalizedPatterns := NormalizeNames(patterns)
	if len(normalizedPatterns) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(available))
	out := make([]string, 0, len(normalizedPatterns))
	for _, patternText := range normalizedPatterns {
		matched := false
		for _, candidate := range available {
			ok, err := path.Match(patternText, candidate)
			if err != nil {
				return nil, fmt.Errorf("invalid %s obfuscator pattern %q: %w", stage, patternText, err)
			}
			if !ok {
				continue
			}
			matched = true
			if _, exists := seen[candidate]; exists {
				continue
			}
			seen[candidate] = struct{}{}
			out = append(out, candidate)
		}
		if !matched {
			return nil, unknownObfuscatorError(stage, patternText, available)
		}
	}
	return out, nil
}

func unknownObfuscatorError(stage, name string, available []string) error {
	if len(available) == 0 {
		return fmt.Errorf("unknown %s obfuscator/pattern %q (no %s obfuscators registered)", stage, name, stage)
	}
	return fmt.Errorf(
		"unknown %s obfuscator/pattern %q (available: %s; glob patterns like '*' are supported)",
		stage,
		name,
		strings.Join(available, ", "),
	)
}

func sortedKeys[T any](items map[string]T) []string {
	out := make([]string, 0, len(items))
	for name := range items {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
