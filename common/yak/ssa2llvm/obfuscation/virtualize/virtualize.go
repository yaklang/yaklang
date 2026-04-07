// Package virtualize implements a body-replace obfuscator that converts
// selected function bodies into a compact bytecode (VM IR) representation
// executed by a lightweight VM at runtime.
//
// It is registered under the name "virtualize" and classified as
// CategoryBodyReplace — at most one body-replace obfuscator may own a function.
package virtualize

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/core"
	vmencode "github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/encode"
	vmlowering "github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/lowering"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/region"
	vmseed "github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/seed"
)

const obfName = "virtualize"

// ObfDataKey is the key under which the virtualize plan is stored in
// core.Context.ObfData. The compiler reads this to emit VM wrapper stubs.
const ObfDataKey = "virtualize:plan"

func init() {
	core.Register(virtualizeObfuscator{})
}

type virtualizeObfuscator struct{}

func (virtualizeObfuscator) Name() string    { return obfName }
func (virtualizeObfuscator) Kind() core.Kind { return core.KindHybrid }

func (v virtualizeObfuscator) Apply(ctx *core.Context) error {
	if ctx == nil {
		return nil
	}
	switch ctx.Stage {
	case core.StageSSAPre:
		return v.applySSAPre(ctx)
	default:
		return nil
	}
}

// ---------- plan types ----------

// Plan holds the result of virtualization lowering, stored in ObfData.
type Plan struct {
	BlobHex         string
	SeedHex         string
	HostBindingSpec string
	ByName          map[string]*VirtualizedFunc
}

// VirtualizedFunc describes a single virtualized function.
type VirtualizedFunc struct {
	Name  string
	Index int
}

// ---------- SSA Pre stage ----------

func (virtualizeObfuscator) applySSAPre(ctx *core.Context) error {
	program := ctx.SSA
	if program == nil {
		return nil
	}

	candidates := collectCandidates(ctx)
	if len(candidates) == 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})

	// Filter out functions that cannot be lowered to PIR.
	var lowerable []region.Candidate
	for _, c := range candidates {
		if region.IsLowerable(c.Func) {
			lowerable = append(lowerable, c)
		}
	}
	if len(lowerable) == 0 {
		return nil
	}
	candidates = lowerable

	// Lower to PIR.
	pirRegion, err := vmlowering.LowerRegion(candidates)
	if err != nil {
		return fmt.Errorf("virtualize: lowering failed: %w", err)
	}

	// Generate seed and encode.
	seed, err := vmseed.Generate()
	if err != nil {
		return fmt.Errorf("virtualize: seed generation failed: %w", err)
	}
	blob, err := vmencode.Encode(pirRegion, seed)
	if err != nil {
		return fmt.Errorf("virtualize: encode failed: %w", err)
	}

	// Build plan.
	plan := &Plan{
		BlobHex:         hex.EncodeToString(blob),
		SeedHex:         hex.EncodeToString(seed.Raw[:]),
		HostBindingSpec: buildHostBindingSpec(pirRegion.HostSymbols),
		ByName:          make(map[string]*VirtualizedFunc, len(pirRegion.Functions)),
	}
	for i, fn := range pirRegion.Functions {
		plan.ByName[fn.Name] = &VirtualizedFunc{Name: fn.Name, Index: i}
	}

	ctx.SetObfData(ObfDataKey, plan)

	// Mark all virtualized functions as body-replaced so downstream obfuscators
	// (e.g. callret) treat them as opaque and do not attempt to inline them.
	for name := range plan.ByName {
		ctx.MarkBodyReplaced(obfName, name)
	}

	return nil
}

func collectCandidates(ctx *core.Context) []region.Candidate {
	program := ctx.SSA
	if program == nil {
		return nil
	}
	var candidates []region.Candidate
	program.EachFunction(func(fn *ssa.Function) {
		if fn == nil || fn.IsExtern() || fn.GetProgram() != program {
			return
		}
		name := fn.GetName()
		if !ctx.IsSelected(obfName, name) {
			return
		}
		candidates = append(candidates, region.Candidate{
			Func:   fn,
			Name:   name,
			Reason: "policy",
		})
	})
	return candidates
}

func buildHostBindingSpec(hostSymbols []string) string {
	if len(hostSymbols) == 0 {
		return ""
	}
	var b strings.Builder
	for _, name := range hostSymbols {
		b.WriteString(name)
		b.WriteByte('\n')
	}
	return b.String()
}
