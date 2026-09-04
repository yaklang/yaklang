package test

import (
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/yak/python/python2ssa"
)

// antlrRacePySamples are structurally diverse on purpose: PredictionContext
// keys off ATN states rather than token text, so near-identical sources all
// reuse the same cached contexts and never exercise concurrent cache inserts.
var antlrRacePySamples = []string{
	"import os\nimport sys\n\ndef f(a):\n    return a + 1\n",
	"from typing import List\n\n\nclass C:\n    attr = 1\n\n    def m(self, *args, **kw):\n        return [x for x in args if x]\n",
	"try:\n    import ujson as json\nexcept ImportError:\n    import json\nfinally:\n    pass\n",
	"with open('f') as fh, open('g', 'w') as out:\n    for line in fh:\n        if line.strip():\n            out.write(line)\n        elif line:\n            out.write('\\n')\n",
	"x = (lambda a, b=2, *c, **d: a if a > b else b)(1)\ny = {k: v for k, v in z.items() if k not in ('a', 'b')}\n",
	"def g():\n    def h():\n        return 1\n    return h\n\n\n@decorator\ndef i():\n    yield from range(10)\n",
	"from . import sibling\nfrom ..pkg import thing\nfrom .. import other\n",
	"if a:\n    pass\nelif b:\n    pass\nelse:\n    while c:\n        break\n    else:\n        pass\n",
}

// TestConcurrentFrontendAndWorkerCacheInit reproduces the CI crash shape:
// uncached Frontend parses (bound to the package-level static ANTLR caches)
// running while new parse workers spin up and build their AntlrCache.
//
// The static caches are only safe because the ANTLR runtime serializes shared
// cache mutation under atn.stateMu, and that holds only while each parser's
// (ATN, DFA, PredictionContextCache) triple comes from a single, immutable
// static generation. Regressing GetPythonParserSerializedATN to the raw init
// function breaks that and aborts with
// "fatal error: concurrent map read and map write" in JMap.Get.
func TestConcurrentFrontendAndWorkerCacheInit(t *testing.T) {
	const rounds = 12
	var start, wg sync.WaitGroup
	start.Add(1)

	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			src := antlrRacePySamples[i%len(antlrRacePySamples)]
			start.Wait()
			for j := 0; j < rounds; j++ {
				if _, err := python2ssa.Frontend(src); err != nil && !strings.Contains(err.Error(), "undefined") {
					t.Errorf("static-path parse failed: %v", err)
					return
				}
			}
		}(i)
	}

	// Worker spin-ups: each parse lane calls GetAntlrCache() exactly once, which
	// is what previously re-ran the full static init.
	builder := python2ssa.CreateBuilder()
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			start.Wait()
			for j := 0; j < rounds; j++ {
				cache := builder.GetAntlrCache()
				if cache == nil {
					t.Errorf("worker cache is nil")
					return
				}
			}
		}(i)
	}

	start.Done()
	wg.Wait()
}
