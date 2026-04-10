package profile_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/profile"
)

func sampleInventory() *profile.Inventory {
	return profile.NewInventory([]profile.FuncInfo{
		{Name: "main", SSAID: 1, BlockCount: 3, InstCount: 20, IsEntry: true},
		{Name: "helper", SSAID: 2, BlockCount: 5, InstCount: 40},
		{Name: "compute", SSAID: 3, BlockCount: 10, InstCount: 100},
		{Name: "tiny", SSAID: 4, BlockCount: 1, InstCount: 3},
		{Name: "medium", SSAID: 5, BlockCount: 4, InstCount: 30},
		{Name: "extern_fn", SSAID: 6, IsExtern: true},
	})
}

func TestResolveNilProfile(t *testing.T) {
	res, err := profile.Resolve(nil, sampleInventory())
	require.NoError(t, err)
	assert.Empty(t, res.Selections)
}

func TestResolveNilInventory(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{{Name: "addsub"}},
	}
	res, err := profile.Resolve(prof, nil)
	require.NoError(t, err)
	assert.Empty(t, res.Selections)
}

func TestResolveAllFunctions(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{{
			Name:     "addsub",
			Category: profile.CategoryLocal,
		}},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)

	funcs := res.FuncsFor("addsub")
	require.NotNil(t, funcs)
	assert.Len(t, funcs, 4)
	assert.Contains(t, funcs, "helper")
	assert.Contains(t, funcs, "compute")
	assert.Contains(t, funcs, "tiny")
	assert.Contains(t, funcs, "medium")
	assert.NotContains(t, funcs, "main")
	assert.NotContains(t, funcs, "extern_fn")
}

func TestResolveAllowEntry(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{{
			Name:     "addsub",
			Category: profile.CategoryLocal,
			Selector: profile.Selector{AllowEntry: true},
		}},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)

	funcs := res.FuncsFor("addsub")
	assert.Len(t, funcs, 5)
	assert.Contains(t, funcs, "main")
}

func TestResolveIncludeGlob(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{{
			Name:     "xor",
			Category: profile.CategoryLocal,
			Selector: profile.Selector{
				Include:    []string{"hel*", "com*"},
				AllowEntry: true,
			},
		}},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)

	funcs := res.FuncsFor("xor")
	assert.Len(t, funcs, 2)
	assert.Contains(t, funcs, "helper")
	assert.Contains(t, funcs, "compute")
}

func TestResolveExcludeGlob(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{{
			Name:     "mba",
			Category: profile.CategoryLocal,
			Selector: profile.Selector{
				Exclude:    []string{"tiny"},
				AllowEntry: true,
			},
		}},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)

	funcs := res.FuncsFor("mba")
	assert.NotContains(t, funcs, "tiny")
	assert.NotContains(t, funcs, "extern_fn")
	assert.Contains(t, funcs, "helper")
	assert.Contains(t, funcs, "main")
}

func TestResolveExcludeOverridesInclude(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{{
			Name:     "xor",
			Category: profile.CategoryLocal,
			Selector: profile.Selector{
				Include:    []string{"*"},
				Exclude:    []string{"helper"},
				AllowEntry: true,
			},
		}},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)

	funcs := res.FuncsFor("xor")
	assert.NotContains(t, funcs, "helper")
	assert.Contains(t, funcs, "compute")
}

func TestResolveMinBlocks(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{{
			Name:     "opaque",
			Category: profile.CategoryLocal,
			Selector: profile.Selector{MinBlocks: 4, AllowEntry: true},
		}},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)

	funcs := res.FuncsFor("opaque")
	assert.Len(t, funcs, 3)
	assert.Contains(t, funcs, "helper")
	assert.Contains(t, funcs, "compute")
	assert.Contains(t, funcs, "medium")
}

func TestResolveMinInsts(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{{
			Name:     "mba",
			Category: profile.CategoryLocal,
			Selector: profile.Selector{MinInsts: 25, AllowEntry: true},
		}},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)

	funcs := res.FuncsFor("mba")
	assert.Len(t, funcs, 3)
	assert.Contains(t, funcs, "helper")
	assert.Contains(t, funcs, "compute")
	assert.Contains(t, funcs, "medium")
}

func TestResolveRatio(t *testing.T) {
	prof := &profile.Profile{
		SelectionSeed: 42,
		Obfuscators: []profile.ObfEntry{{
			Name:     "addsub",
			Category: profile.CategoryLocal,
			Selector: profile.Selector{Ratio: ptrFloat(0.5)},
		}},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)

	funcs := res.FuncsFor("addsub")
	assert.Len(t, funcs, 2)
}

func TestResolveRatioDeterministic(t *testing.T) {
	prof := &profile.Profile{
		SelectionSeed: 123,
		Obfuscators: []profile.ObfEntry{{
			Name:     "addsub",
			Category: profile.CategoryLocal,
			Selector: profile.Selector{Ratio: ptrFloat(0.5)},
		}},
	}
	inv := sampleInventory()

	res1, err := profile.Resolve(prof, inv)
	require.NoError(t, err)
	res2, err := profile.Resolve(prof, inv)
	require.NoError(t, err)
	assert.Equal(t, res1.Selections["addsub"], res2.Selections["addsub"])
}

func TestResolveCount(t *testing.T) {
	prof := &profile.Profile{
		SelectionSeed: 7,
		Obfuscators: []profile.ObfEntry{{
			Name:     "xor",
			Category: profile.CategoryLocal,
			Selector: profile.Selector{Count: ptrInt(1)},
		}},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)
	assert.Len(t, res.FuncsFor("xor"), 1)
}

func TestResolveCountClampsToAvailable(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{{
			Name:     "addsub",
			Category: profile.CategoryLocal,
			Selector: profile.Selector{Count: ptrInt(100), AllowEntry: true},
		}},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)
	assert.Len(t, res.FuncsFor("addsub"), 5)
}

func TestResolveCountZero(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{{
			Name:     "addsub",
			Category: profile.CategoryLocal,
			Selector: profile.Selector{Count: ptrInt(0)},
		}},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)
	assert.Empty(t, res.FuncsFor("addsub"))
}

func TestResolveRatioZero(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{{
			Name:     "addsub",
			Category: profile.CategoryLocal,
			Selector: profile.Selector{Ratio: ptrFloat(0.0)},
		}},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)
	assert.Empty(t, res.FuncsFor("addsub"))
}

func TestResolveRatioOne(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{{
			Name:     "addsub",
			Category: profile.CategoryLocal,
			Selector: profile.Selector{Ratio: ptrFloat(1.0)},
		}},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)
	assert.Len(t, res.FuncsFor("addsub"), 4)
}

func TestResolveBodyReplaceConflict(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{
			{
				Name:     "virtualize",
				Category: profile.CategoryBodyReplace,
				Selector: profile.Selector{AllowEntry: true},
			},
			{
				Name:     "other_body_replace",
				Category: profile.CategoryBodyReplace,
				Selector: profile.Selector{AllowEntry: true},
			},
		},
	}
	_, err := profile.Resolve(prof, sampleInventory())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "claimed by body-replace obfuscator")
}

func TestResolveBodyReplaceNoConflictWhenDisjoint(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{
			{
				Name:     "virtualize",
				Category: profile.CategoryBodyReplace,
				Selector: profile.Selector{Include: []string{"helper"}},
			},
			{
				Name:     "other_body_replace",
				Category: profile.CategoryBodyReplace,
				Selector: profile.Selector{Include: []string{"compute"}},
			},
		},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)
	assert.Contains(t, res.FuncsFor("virtualize"), "helper")
	assert.Contains(t, res.FuncsFor("other_body_replace"), "compute")
}

func TestResolveDifferentObfKindsCanShareFunctions(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{
			{
				Name:     "callret",
				Category: profile.CategoryCallflow,
			},
			{
				Name:     "addsub",
				Category: profile.CategoryLocal,
			},
		},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)
	assert.Contains(t, res.FuncsFor("callret"), "helper")
	assert.Contains(t, res.FuncsFor("addsub"), "helper")
}

func TestResolveMultipleObfuscatorsDeterministicOrder(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{
			{Name: "addsub", Category: profile.CategoryLocal},
			{Name: "xor", Category: profile.CategoryLocal},
			{Name: "mba", Category: profile.CategoryLocal},
		},
	}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)
	for _, name := range []string{"addsub", "xor", "mba"} {
		assert.NotEmpty(t, res.FuncsFor(name), "expected functions for %s", name)
	}
}

func TestResolveSeedOverridePerObfuscator(t *testing.T) {
	prof := &profile.Profile{
		SelectionSeed: 42,
		Obfuscators: []profile.ObfEntry{{
			Name:     "addsub",
			Category: profile.CategoryLocal,
			Selector: profile.Selector{
				Ratio: ptrFloat(0.5),
				Seed:  99,
			},
		}},
	}
	inv := sampleInventory()

	res1, err := profile.Resolve(prof, inv)
	require.NoError(t, err)

	prof.SelectionSeed = 999
	res2, err := profile.Resolve(prof, inv)
	require.NoError(t, err)
	assert.Equal(t, res1.Selections["addsub"], res2.Selections["addsub"])
}

func TestResolveFuncsForMissing(t *testing.T) {
	res := &profile.Resolution{
		Selections: map[string]map[string]struct{}{},
	}
	assert.Nil(t, res.FuncsFor("nonexistent"))
}

func TestResolveFuncsForNilResolution(t *testing.T) {
	var res *profile.Resolution
	assert.Nil(t, res.FuncsFor("anything"))
}

func TestInventoryLookup(t *testing.T) {
	inv := sampleInventory()
	fi := inv.Lookup("helper")
	require.NotNil(t, fi)
	assert.Equal(t, int64(2), fi.SSAID)
	assert.Equal(t, 5, fi.BlockCount)
	assert.Nil(t, inv.Lookup("nonexistent"))
}

func TestInventoryLookupNil(t *testing.T) {
	var inv *profile.Inventory
	assert.Nil(t, inv.Lookup("anything"))
}

func TestResolveEmptyProfile(t *testing.T) {
	prof := &profile.Profile{}
	res, err := profile.Resolve(prof, sampleInventory())
	require.NoError(t, err)
	assert.Empty(t, res.Selections)
}

func TestResolveSelectorSeedField(t *testing.T) {
	prof := &profile.Profile{
		Obfuscators: []profile.ObfEntry{{
			Name:     "addsub",
			Category: profile.CategoryLocal,
			Selector: profile.Selector{
				Ratio: ptrFloat(0.5),
				Seed:  42,
			},
		}},
	}
	inv := sampleInventory()
	res1, err := profile.Resolve(prof, inv)
	require.NoError(t, err)

	prof.Obfuscators[0].Selector.Seed = 43
	res2, err := profile.Resolve(prof, inv)
	require.NoError(t, err)

	assert.NotEqual(t, res1.Selections["addsub"], res2.Selections["addsub"])
}
