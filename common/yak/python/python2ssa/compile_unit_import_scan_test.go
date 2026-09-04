package python2ssa

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func pyEdgeKey(e pyImportEdge) string {
	return e.kind.String() + "|" + e.dots + "|" + e.module + "|" + e.pkg
}

func scanKeys(src string) []string {
	edges := scanPyImportEdges(src)
	keys := make([]string, 0, len(edges))
	for _, e := range edges {
		keys = append(keys, pyEdgeKey(e))
	}
	sort.Strings(keys)
	return keys
}

func requireScan(t *testing.T, src string, want ...string) {
	t.Helper()
	got := scanKeys(src)
	expected := append([]string(nil), want...)
	sort.Strings(expected)
	if strings.Join(got, ",") != strings.Join(expected, ",") {
		t.Fatalf("edges mismatch\n got: %v\nwant: %v\nsrc:\n%s", got, expected, src)
	}
}

// TestScanPyImportEdgesCorpusForms pins the syntax forms found in the real
// Python corpora used for SSA regression work (Apache Kafka kafkatest, Apache
// TinkerPop gremlin_python, Apache Ranger, CRMEB and the wanwu RAG/agent
// trees). Each case is a construct the plan-phase scanner must keep resolving
// after the move away from parsing every file.
func TestScanPyImportEdgesCorpusForms(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "kafka config.py bare relative import",
			src:  "from . import config_property\n",
			want: []string{"relative|.||"},
		},
		{
			name: "tinkerpop two-level relative import",
			src:  "from .. import statics\n",
			want: []string{"relative|..||"},
		},
		{
			name: "wanwu parenthesized relative import",
			src:  "from .base import (\n    DialogueWithSharedMemoryChains\n)\n\n__all__ = [\n    \"DialogueWithSharedMemoryChains\"\n]\n",
			want: []string{"relative|.|base|"},
		},
		{
			name: "wanwu parenthesized absolute imports",
			src:  "from models.base.base import (\n    BaseLLM,\n)\nfrom models.base.remote_rpc_model import (\n    RemoteModel,\n)\n",
			want: []string{
				"absolute||models.base.base|",
				"absolute||models.base.remote_rpc_model|",
			},
		},
		{
			name: "tinkerpop indented try/except import split across lines",
			src:  "        if transport_factory is None:\n            try:\n                from gremlin_python.driver.aiohttp.transport import (\n                    AiohttpTransport, AiohttpHTTPTransport)\n            except ImportError:\n                raise Exception(\"Please install AIOHTTP or pass \"\n                                \"custom transport factory\")\n",
			want: []string{"absolute||gremlin_python.driver.aiohttp.transport|"},
		},
		{
			name: "aliased from-imports including dunder names",
			src:  "from kafkatest import __version__ as __kafkatest_version__\nfrom gremlin_python.process.graph_traversal import __ as AnonymousTraversal\nfrom xml.etree import ElementTree as ET\nimport getopt as opts\nimport models.shared as shared\n",
			want: []string{
				"absolute||getopt|",
				"absolute||gremlin_python.process.graph_traversal|",
				"absolute||kafkatest|",
				"absolute||models.shared|",
				"absolute||xml.etree|",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireScan(t, tt.src, tt.want...)
		})
	}
}

// TestScanPyImportEdgesIgnoresNonCode covers import-like text that is not a
// statement. The license headers in the corpus put double quotes inside
// comments and the wanwu/kafka modules open with docstrings, so both a quote
// cascade across comments and a zero-column import inside a docstring must be
// ignored.
func TestScanPyImportEdgesIgnoresNonCode(t *testing.T) {
	license := "# Licensed to the Apache Software Foundation (ASF) under one\n" +
		"# or more contributor license agreements.  See the NOTICE file\n" +
		"# \"License\"); you may not use this file except in compliance\n" +
		"# with the License.  You may obtain a copy of the License at\n" +
		"#\n" +
		"#   http://www.apache.org/licenses/LICENSE-2.0\n" +
		"#\n" +
		"# \"AS IS\" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY\n" +
		"\"\"\"\n" +
		"import os\n" +
		"from .mod import helper\n" +
		"\"\"\"\n"
	requireScan(t, license)

	requireScan(t, "import io  # import logging\n", "absolute||io|")
	requireScan(t, "msg = \"import os\"\nother = 'from .mod import f'\n")
	requireScan(t, "def f():\n    return \"\"\"\nimport os\nfrom .mod import f\n\"\"\"\n")
	// An unterminated quote must not swallow the rest of the file: the literal
	// ends at end of line, so the import on the next line is still a statement.
	requireScan(t, "text = \"unterminated\nimport os\nfrom .mod import f\n",
		"absolute||os|", "relative|.|mod|")
	requireScan(t, "s = \"\\\"quoted inner\\\"\"\nimport os\n", "absolute||os|")
	requireScan(t, "import io  # noqa\nimport os\n", "absolute||io|", "absolute||os|")
}

// TestScanPyImportEdgesDynamicGuards keeps the guards the AST walk had for
// dynamic imports: only a literal first argument on __import__, import_module
// or importlib.import_module counts.
func TestScanPyImportEdgesDynamicGuards(t *testing.T) {
	requireScan(t, "mod = importlib.import_module(\"db.engine\")\n", "dynamic||db.engine|")
	requireScan(t, "mod = __import__('db.utils')\n", "dynamic||db.utils|")
	requireScan(t, "mod = import_module(\"a.b\")\n", "dynamic||a.b|")
	requireScan(t, "mod = importlib.import_module(\".base\", package=\"pkg\")\n", "dynamic||.base|pkg")
	requireScan(t, "mod = importlib.import_module(\".base\",package='pkg')\n", "dynamic||.base|pkg")
	requireScan(t, "mod = importlib.import_module(name)\n")
	requireScan(t, "mod = import_module(f\"db.engine\")\n")
	requireScan(t, "mod = myobj.import_module(\"db.engine\")\n")
	requireScan(t, "mod = a.b.import_module(\"db.engine\")\n")
	requireScan(t, "mod = compat.__import__(\"db.engine\")\n")
	requireScan(t, "value = 1\nimport_moduleX(\"db.engine\")\n")
	requireScan(t, "mod = import_module(\"db.engine\")  # import_module(\"other\")\n", "dynamic||db.engine|")
}

// TestScanPyImportEdgesStatementForms covers statement shapes the scanner walks
// line by line and splits on semicolons.
func TestScanPyImportEdgesStatementForms(t *testing.T) {
	requireScan(t, "import os,errno,sys,getopt\n",
		"absolute||errno|", "absolute||getopt|", "absolute||os|", "absolute||sys|")
	requireScan(t, "import os, sys\nimport pwd, grp\n",
		"absolute||grp|", "absolute||os|", "absolute||pwd|", "absolute||sys|")
	requireScan(t, "import os; from bdir import b\n", "absolute||bdir|", "absolute||os|")
	requireScan(t, "from .mod import a; from ..other import b\n", "relative|.|mod|", "relative|..|other|")
	requireScan(t, "from a import *\n", "absolute||a|")
	requireScan(t, "\tfrom . import sibling\n", "relative|.||")
	requireScan(t, "from ..pkg.sub import deep\n", "relative|..|pkg.sub|")
	requireScan(t, "importlib.reload(mod)\n")
	requireScan(t, "fromstring = 1\nimporter = 2\n")
	requireScan(t, "")
}

// TestScanPyImportEdgesCorpusParity re-scans every .py file under a local
// corpus and compares the result with expectations produced by Python's own ast
// module. It is opt-in because the corpus lives outside the repository:
//
//	YAK_PYTHON_IMPORT_CORPUS=/path/to/python/files
//	YAK_PYTHON_IMPORT_ORACLE=/path/to/expected.tsv
//
// The oracle TSV holds one edge per line:
// <relpath>\t<kind>\t<dots>\t<module>\t<pkg>
func TestScanPyImportEdgesCorpusParity(t *testing.T) {
	root := os.Getenv("YAK_PYTHON_IMPORT_CORPUS")
	oraclePath := os.Getenv("YAK_PYTHON_IMPORT_ORACLE")
	if root == "" || oraclePath == "" {
		t.Skip("set YAK_PYTHON_IMPORT_CORPUS and YAK_PYTHON_IMPORT_ORACLE to run corpus parity")
	}
	raw, err := os.ReadFile(oraclePath)
	if err != nil {
		t.Fatal(err)
	}
	want := make(map[string]map[string]bool)
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimRight(line, "\r")
		parts := strings.Split(line, "\t")
		if line == "" || len(parts) != 5 {
			continue
		}
		if want[parts[0]] == nil {
			want[parts[0]] = make(map[string]bool)
		}
		want[parts[0]][strings.Join(parts[1:], "|")] = true
	}
	var scanned, missing, extra int
	var samples []string
	for rel, wantSet := range want {
		bs, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		scanned++
		got := make(map[string]bool)
		for _, k := range scanKeys(string(bs)) {
			got[k] = true
		}
		for k := range wantSet {
			if !got[k] {
				missing++
				if len(samples) < 20 {
					samples = append(samples, "missing "+rel+" "+k)
				}
			}
		}
		for k := range got {
			if !wantSet[k] {
				extra++
				if len(samples) < 20 {
					samples = append(samples, "extra "+rel+" "+k)
				}
			}
		}
	}
	for _, s := range samples {
		t.Error(s)
	}
	t.Logf("corpus parity: files=%d missing=%d extra=%d", scanned, missing, extra)
	if missing != 0 || extra != 0 {
		t.Fatalf("corpus parity broken: missing=%d extra=%d", missing, extra)
	}
}
