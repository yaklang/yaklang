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
// pyEdgeKeys renders the edge set as "from->to" keys.
func pyEdgeKeys(edges []ssa.UnitRef) map[string]bool {
	keys := map[string]bool{}
	for _, e := range edges {
		keys[edgeKey(e)] = true
	}
	return keys
}

// pyEdgeRaws collects the Raw fields of all edges.
func pyEdgeRaws(edges []ssa.UnitRef) map[string]bool {
	raws := map[string]bool{}
	for _, e := range edges {
		raws[e.Raw] = true
	}
	return raws
}

func TestPythonCompileUnit_ParenthesizedMultilineImportEdges(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("pkg/__init__.py", "")
	vf.AddFile("pkg/moda.py", "def f():\n    return 1\n")
	vf.AddFile("pkg/modb.py", "def g():\n    return 2\n")
	vf.AddFile("pkg/user.py", "from . import (\n    moda,\n    modb,\n)\nfrom .moda import (\n    f as helper,\n)\n")

	edges := requirePyUnitEdges(t, vf)
	got := pyEdgeKeys(edges)
	// "from . import (moda, modb)" resolves each imported name as a submodule
	// of the importing package — but the units here are directories, so both
	// moda and modb live in the same pkg unit as user.py: no cross-unit edge.
	// "from .moda import f" also stays inside the pkg unit.
	require.Empty(t, filterEdgesNotWithin(edges, "dir:pkg"), "unexpected cross-unit edges: %v", edges)

	// With a second package importing via parentheses, the edge must appear.
	vf.AddFile("other/__init__.py", "")
	vf.AddFile("other/__init2__.py", "")
	vf.AddFile("other/user2.py", "from pkg import (\n    moda,\n)\n")
	edges = requirePyUnitEdges(t, vf)
	got = pyEdgeKeys(edges)
	require.Contains(t, got, "dir:other->dir:pkg", "edges: %v", edges)
}

// filterEdgesNotWithin returns edges whose From unit differs from the given unit.
func filterEdgesNotWithin(edges []ssa.UnitRef, unitKey string) []ssa.UnitRef {
	var out []ssa.UnitRef
	for _, e := range edges {
		if e.From != unitKey {
			out = append(out, e)
		}
	}
	return out
}

func TestPythonCompileUnit_CommentAndStringImportsProduceNoEdge(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("app/__init__.py", "")
	vf.AddFile("app/mod.py", "def f():\n    return 1\n")
	// The only import-like text lives in a comment, a plain string, and a
	// docstring — none may produce an edge. With the old regex scanner the
	// comment and string lines both matched.
	vf.AddFile("app/main.py", `# from .mod import f
s = "from .mod import f"

def f():
	"""from .mod import f"""
	return s

# import app.mod
t = 'import app.mod'
`)

	edges := requirePyUnitEdges(t, vf)
	require.Empty(t, edges, "imports in comments/strings must not produce edges: %v", edges)
}

func TestPythonCompileUnit_SemicolonSameLineImportEdges(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("bdir/__init__.py", "")
	vf.AddFile("bdir/b.py", "def f():\n    return 1\n")
	vf.AddFile("a.py", "import os; from bdir import b\n")

	edges := requirePyUnitEdges(t, vf)
	got := pyEdgeKeys(edges)
	require.Contains(t, got, "dir:.->dir:bdir", "edges: %v", edges)
	// "os" does not exist in the project: no edge for it.
	require.NotContains(t, got, "dir:.->dir:.", "edges: %v", edges)
}

func TestPythonCompileUnit_SyntaxErrorFallsBackToRegex(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("bdir/__init__.py", "")
	vf.AddFile("bdir/b.py", "def f():\n    return 1\n")
	// Unbalanced parenthesis: the file fails to parse, so the legacy regex
	// fallback must still produce the edge.
	vf.AddFile("a.py", "def broken(:\n    pass\nfrom bdir import b\n")

	edges := requirePyUnitEdges(t, vf)
	got := pyEdgeKeys(edges)
	require.Contains(t, got, "dir:.->dir:bdir", "syntax-error file must keep its edges via the regex fallback: %v", edges)
}

func TestPythonCompileUnit_DynamicImportExcludesNonLiteral(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("db/__init__.py", "")
	vf.AddFile("db/engine.py", "")
	vf.AddFile("main.py", `
import importlib
from importlib import import_module

name = "db.engine"
mod1 = importlib.import_module(name)   # non-literal: no edge
mod2 = import_module(f"db.engine")     # f-string: no edge
mod3 = myobj.import_module("db.engine")  # wrong receiver: no edge
mod4 = __import__("db.engine")         # literal: edge
`)

	edges := requirePyUnitEdges(t, vf)
	raws := pyEdgeRaws(edges)
	require.Contains(t, raws, "db.engine", "edges: %v", edges)
	// All forms collapse onto the single dir:db unit; only one edge remains.
	got := pyEdgeKeys(edges)
	require.Contains(t, got, "dir:.->dir:db", "edges: %v", edges)
	require.Equal(t, 1, len(edges), "only the literal __import__ may produce an edge: %v", edges)
}
