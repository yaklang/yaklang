package ssatest

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// TestRecompileClearCache tests that program cache is cleared after recompilation
func TestRecompileClearCache(t *testing.T) {
	// Create a simple yak program
	progName := uuid.NewString()
	code1 := `
a = 1
println(a)
`

	// First compilation
	prog1, err := ssaapi.Parse(code1,
		ssaapi.WithProgramName(progName),
		ssaapi.WithLanguage(ssaconfig.TS),
	)
	require.NoError(t, err)
	require.NotNil(t, prog1)

	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progName)
	}()

	// Load from database - this will cache it in ProgramCache
	prog2, err := ssaapi.FromDatabase(progName)
	require.NoError(t, err)
	require.NotNil(t, prog2)

	// Load from database again - should be cached (same instance)
	prog2_again, err := ssaapi.FromDatabase(progName)
	require.NoError(t, err)
	require.NotNil(t, prog2_again)
	require.True(t, prog2 == prog2_again, "Expected prog2_again to be cached version of prog2")

	// Recompile with different code
	code2 := `
a = 1
b = 2
c = a + b
println(c)
`

	// Simulate recompilation by deleting IR code and re-parsing
	ssadb.DeleteProgramIrCode(ssadb.GetDB(), progName)
	ssaapi.ProgramCache.Remove(progName)

	_, err = ssaapi.Parse(code2,
		ssaapi.WithProgramName(progName),
		ssaapi.WithLanguage(ssaconfig.TS),
	)
	require.NoError(t, err)

	// Load from database again - should get new version (cache was cleared by DeleteProgramIrCode)
	prog3, err := ssaapi.FromDatabase(progName)
	require.NoError(t, err)
	require.NotNil(t, prog3)

	// Check that prog3 is NOT the same instance as prog2 (cache was cleared)
	require.True(t, prog2 != prog3, "Expected prog3 to be different from prog2 after recompile")
}

// TestRecompileSyntaxFlowUsesNewIR tests that SyntaxFlow queries use new IR after recompilation
func TestRecompileSyntaxFlowUsesNewIR(t *testing.T) {
	// Create a simple yak program
	progName := uuid.NewString()
	code1 := `
target_var = "old_value"
println(target_var)
`

	// First compilation
	prog1, err := ssaapi.Parse(code1,
		ssaapi.WithProgramName(progName),
		ssaapi.WithLanguage(ssaconfig.TS),
	)
	require.NoError(t, err)
	require.NotNil(t, prog1)

	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progName)
	}()

	// Run SyntaxFlow query
	result1, err := prog1.SyntaxFlowWithError("target_var as $result")
	require.NoError(t, err)
	require.NotNil(t, result1)

	// Get the value
	values1 := result1.GetValues("result")
	require.Greater(t, len(values1), 0, "Should find target_var in first compilation")

	// Check that we can find the old value in the source code
	found1 := false
	for _, v := range values1 {
		src := v.StringWithSourceCode()
		if strings.Contains(src, "old_value") {
			found1 = true
			break
		}
	}
	require.True(t, found1, "Expected to find 'old_value' in first compilation")

	// Recompile with different code
	code2 := `
target_var = "new_value"
println(target_var)
`

	// Delete IR code and recompile
	ssadb.DeleteProgramIrCode(ssadb.GetDB(), progName)

	_, err = ssaapi.Parse(code2,
		ssaapi.WithProgramName(progName),
		ssaapi.WithLanguage(ssaconfig.TS),
	)
	require.NoError(t, err)

	// Load from database - should get new version
	prog2, err := ssaapi.FromDatabase(progName)
	require.NoError(t, err)
	require.NotNil(t, prog2)

	// Run SyntaxFlow query again
	result2, err := prog2.SyntaxFlowWithError("target_var as $result")
	require.NoError(t, err)
	require.NotNil(t, result2)

	// Get the value
	values2 := result2.GetValues("result")
	require.Greater(t, len(values2), 0, "Should find target_var after recompilation")

	// Check that we can find the NEW value (not the old one)
	found2 := false
	foundOld := false
	for _, v := range values2 {
		src := v.StringWithSourceCode()
		if strings.Contains(src, "new_value") {
			found2 = true
		}
		if strings.Contains(src, "old_value") {
			foundOld = true
		}
	}
	require.True(t, found2, "Expected to find 'new_value' after recompilation")
	require.False(t, foundOld, "Should NOT find 'old_value' after recompilation")
}

// TestFromDatabaseReloadsWhenCompileGenerationChanges covers the CI yak-grpc
// case: another process rewrites IR without this process calling
// ProgramCache.Remove. The compile-generation counter is the authoritative
// signal (review A11); a metadata-only updated_at change must NOT drop the
// IR cache.
func TestFromDatabaseReloadsWhenCompileGenerationChanges(t *testing.T) {
	progName := uuid.NewString()
	prog1, err := ssaapi.Parse(`a = 1`,
		ssaapi.WithProgramName(progName),
		ssaapi.WithLanguage(ssaconfig.Yak),
	)
	require.NoError(t, err)
	require.NotNil(t, prog1)
	defer ssadb.DeleteProgram(ssadb.GetDB(), progName)

	cached, err := ssaapi.FromDatabase(progName)
	require.NoError(t, err)
	again, err := ssaapi.FromDatabase(progName)
	require.NoError(t, err)
	require.True(t, cached == again, "same IR revision should stay cached")

	// Metadata-only update (updated_at changes, generation unchanged) must
	// keep the cached IR: the counter, not the timestamp, drives freshness.
	require.NoError(t, ssadb.GetDB().Model(&ssadb.IrProgram{}).
		Where("program_name = ?", progName).
		Update("updated_at", time.Now().Add(2*time.Second)).Error)
	againMeta, err := ssaapi.FromDatabase(progName)
	require.NoError(t, err)
	require.True(t, cached == againMeta, "metadata-only updated_at change must keep cache")

	ir, err := ssadb.GetProgram(progName, ssadb.Application)
	require.NoError(t, err)
	require.NoError(t, ssadb.GetDB().Model(&ssadb.IrProgram{}).
		Where("program_name = ?", progName).
		Update("compile_generation", ir.CompileGeneration+1).Error)

	reloaded, err := ssaapi.FromDatabase(progName)
	require.NoError(t, err)
	require.True(t, cached != reloaded, "FromDatabase must drop cache when compile_generation changes")
}

// TestFromDatabaseStaleOnCompileGeneration proves the compile-generation
// business field drives cache invalidation: when another process bumps
// compile_generation (and deletes IR rows), a cached FromDatabase program
// must be replaced instead of scanning deleted instruction IDs (review A11).
func TestFromDatabaseStaleOnCompileGeneration(t *testing.T) {
	progName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), progName)

	_, err := ssaapi.Parse("a = 1\nprintln(a)",
		ssaapi.WithProgramName(progName),
		ssaapi.WithLanguage(ssaconfig.TS),
	)
	require.NoError(t, err)

	ir, err := ssadb.GetProgram(progName, ssadb.Application)
	require.NoError(t, err)
	require.GreaterOrEqual(t, ir.CompileGeneration, int64(1),
		"compile must bump the program's compile generation")

	prog1, err := ssaapi.FromDatabase(progName)
	require.NoError(t, err)
	require.NotNil(t, prog1)

	// Simulate another process recompiling: delete IR rows and bump the
	// generation directly (not through UpdateProgramWithError, which would
	// bump again).
	ssadb.DeleteProgramIrCode(ssadb.GetDB(), progName)
	require.NoError(t, ssadb.GetDB().Model(&ssadb.IrProgram{}).
		Where("program_name = ?", progName).
		Update("compile_generation", ir.CompileGeneration+1).Error)

	prog2, err := ssaapi.FromDatabase(progName)
	require.NoError(t, err)
	require.True(t, prog1 != prog2,
		"a compile-generation bump must invalidate the cached program")
}
