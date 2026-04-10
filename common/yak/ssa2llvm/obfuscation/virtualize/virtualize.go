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

func init() {
	core.Register(virtualizeObfuscator{})
}

type virtualizeObfuscator struct{}

func (virtualizeObfuscator) Name() string    { return obfName }
func (virtualizeObfuscator) Kind() core.Kind { return core.KindSSA }

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

	blobHex := hex.EncodeToString(blob)
	seedHex := hex.EncodeToString(seed.Raw[:])
	hostBindingSpec := buildHostBindingSpec(pirRegion.HostSymbols)
	for _, fn := range pirRegion.Functions {
		if err := ctx.RegisterFunctionWrapper(&core.FunctionWrapper{
			Owner:         obfName,
			FuncName:      fn.Name,
			RuntimeSymbol: "yak_runtime_invoke_vm",
			Payload:       []string{blobHex, seedHex, fn.Name, hostBindingSpec},
		}); err != nil {
			return fmt.Errorf("virtualize: register wrapper for %q: %w", fn.Name, err)
		}
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
			Reason: "profile-selection",
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
