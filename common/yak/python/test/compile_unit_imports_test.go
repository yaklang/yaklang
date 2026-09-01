package test

import (
	"io/fs"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	python2ssa "github.com/yaklang/yaklang/common/yak/python/python2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// requirePyUnitEdges partitions the virtual FS into Python compile units and
// extracts dependency edges via the python2ssa UnitPartitioner.
func requirePyUnitEdges(t *testing.T, vf *filesys.VirtualFS) []ssa.UnitRef {
	t.Helper()

	builder := python2ssa.CreateBuilder()
	partitioner, ok := builder.(ssa.UnitPartitioner)
	require.True(t, ok, "python2ssa builder must implement ssa.UnitPartitioner")

	files := []string{}
	err := filesys.Recursive(".", filesys.WithFileSystem(vf), filesys.WithStat(func(isDir bool, pathname string, info fs.FileInfo) error {
		if !isDir {
			files = append(files, pathname)
		}
		return nil
	}))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	units := partitioner.PartitionCompileUnits(vf, files)
	require.NotEmpty(t, units)
	return partitioner.CompileUnitDependencies(vf, units)
}

// edgeKey renders an edge as "from->to" for assertions.
func edgeKey(e ssa.UnitRef) string {
	return e.From + "->" + e.To
}

func TestPythonCompileUnit_RelativeImportEdges(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("app/__init__.py", "")
	vf.AddFile("app/mod.py", "def helper():\n    return 1\n")
	vf.AddFile("app/main.py", "from .mod import helper\n")
	vf.AddFile("app/sub/__init__.py", "")
	vf.AddFile("app/sub/deep.py", "from ..mod import helper\nfrom . import sibling\n")
	vf.AddFile("app/sub/sibling.py", "pass\n")
	vf.AddFile("root.py", "from app.sub import deep\n")

	edges := requirePyUnitEdges(t, vf)
	got := map[string]bool{}
	for _, e := range edges {
		got[edgeKey(e)] = true
	}

	// Units are directories: "from .mod import" stays inside the app unit
	// (no edge), while "from ..mod import" in app/sub ascends to app —
	// resolving to the app unit, not app/mod, because a unit owns its files.
	require.NotContains(t, got, "dir:app->dir:app/mod", "edges: %v", edges)
	require.Contains(t, got, "dir:app/sub->dir:app", "edges: %v", edges)
	// Absolute import from the root unit into the nested package.
	require.Contains(t, got, "dir:.->dir:app/sub", "edges: %v", edges)
}

func TestPythonCompileUnit_ImportlibDynamicImportEdges(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
import importlib
from importlib import import_module

mod1 = importlib.import_module("db.engine")
mod2 = import_module("db.models")
mod3 = __import__("db.utils")
`)
	vf.AddFile("db/__init__.py", "")
	vf.AddFile("db/engine.py", "")
	vf.AddFile("db/models.py", "")
	vf.AddFile("db/utils.py", "")

	edges := requirePyUnitEdges(t, vf)
	got := map[string]bool{}
	for _, e := range edges {
		got[edgeKey(e)] = true
	}

	// Units are directories, so all db/* modules collapse into the db unit.
	require.Contains(t, got, "dir:.->dir:db", "edges: %v", edges)
	// Each dynamic form contributes at least one distinct raw edge.
	raws := map[string]bool{}
	for _, e := range edges {
		raws[e.Raw] = true
	}
	require.Contains(t, raws, "db.engine", "edges: %v", edges)
	require.Contains(t, raws, "db.models", "edges: %v", edges)
	require.Contains(t, raws, "db.utils", "edges: %v", edges)
}

func TestPythonCompileUnit_RelativeDynamicImportEdge(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("pkg/__init__.py", "")
	vf.AddFile("pkg/base.py", "def f():\n    return 1\n")
	vf.AddFile("pkg/app.py", "import pkg2.api\n")
	vf.AddFile("pkg2/__init__.py", "")
	vf.AddFile("pkg2/api.py", `
import importlib

mod = importlib.import_module(".base", package="pkg")
`)

	edges := requirePyUnitEdges(t, vf)
	got := map[string]bool{}
	for _, e := range edges {
		got[edgeKey(e)] = true
	}

	// import_module(".base", package="pkg") resolves to pkg: the pkg2 unit
	// gains an edge onto the pkg unit.
	require.Contains(t, got, "dir:pkg2->dir:pkg", "edges: %v", edges)
}

func TestPythonProjectCompile_ImaginaryNumberLiteral(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
import cmath

z = 3j
w = 2.5J
result = cmath.sqrt(z) + w
sink(result)

def sink(v):
    return v
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonCompileUnit_EdgeStability(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("a.py", "import bdir.b\n")
	vf.AddFile("bdir/__init__.py", "")
	vf.AddFile("bdir/b.py", "def f():\n    return 1\n")

	edges := requirePyUnitEdges(t, vf)
	require.NotEmpty(t, edges)
	// DedupeUnitRefs guarantees sorted, deduplicated output.
	sorted := append([]ssa.UnitRef(nil), edges...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].From != sorted[j].From {
			return sorted[i].From < sorted[j].From
		}
		return sorted[i].To < sorted[j].To
	})
	for i := range edges {
		require.Equal(t, sorted[i].From, edges[i].From)
		require.Equal(t, sorted[i].To, edges[i].To)
	}
}