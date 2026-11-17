package sfpattern

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestMatchRegexp_AWSKey(t *testing.T) {
	files := map[string]string{
		"config.env": `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
OTHER=1
`,
		"ok.txt": "no secrets here",
	}
	hits, err := MatchRegexpHits(files, "*", []string{`AKIA[0-9A-Z]{16}`})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Contains(t, hits[0].Text, "AKIA")
}

func TestExecRule_SourceMode_NoSSA(t *testing.T) {
	ruleContent := `
desc(
  mode: "source",
  language: "general",
  title: "aws akia",
  rule_id: "test-aws-akia",
  level: "critical",
)
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit for {
  level: "critical",
  message: "aws key",
}
`
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(ruleContent)
	require.NoError(t, err)
	require.True(t, sfvm.FrameIsSourceMode(frame))

	files := map[string]string{
		"a.properties": `key=AKIAIOSFODNN7EXAMPLE`,
	}
	result, err := ExecFrame(frame, files)
	require.NoError(t, err)
	require.True(t, HasAlert(result))
	require.GreaterOrEqual(t, AlertCount(result), 1)
}

func TestExecFrameOnFS_PositiveNegative(t *testing.T) {
	ruleContent := `
desc(
  mode: "source",
  language: "general",
  title: "private key pem",
  level: "critical",
  "file://bad.pem": <<<POS
-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA0Z3VS5JJcds3xfn/ygWyF6PZGFw
-----END RSA PRIVATE KEY-----
POS,
  "safefile://good.txt": <<<NEG
public key material only
ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC
NEG,
)
${*}.re(/-----BEGIN (RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----/) as $hit
alert $hit for { level: "critical", message: "pem" }
`
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(ruleContent)
	require.NoError(t, err)
	require.True(t, sfvm.FrameIsSourceMode(frame))

	pos, err := frame.ExtractVerifyFilesystemAndLanguage()
	require.NoError(t, err)
	require.NotEmpty(t, pos)
	result, err := ExecFrameOnFS(frame, pos[0].GetVirtualFs())
	require.NoError(t, err)
	require.True(t, HasAlert(result))

	neg, err := frame.ExtractNegativeFilesystemAndLanguage()
	require.NoError(t, err)
	require.NotEmpty(t, neg)
	negResult, err := ExecFrameOnFS(frame, neg[0].GetVirtualFs())
	if err == nil {
		require.False(t, HasAlert(negResult))
	}
}

func TestLoadFilesFromFSWithOptions_SourceExtensions(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	for _, name := range []string{
		"code/Main.scala", "code/Token.sol", "code/lib.ml", "code/lib.mli",
		"code/core.clj", "code/app.ex", "code/app.exs", "code/Page.cls",
		"code/Handler.apex", "code/Page.page-meta.xml",
		"code/init.lua",
	} {
		vfs.AddFile(name, "content")
	}
	// Not in the common source/config allow-list.
	vfs.AddFile("binary.dat", "content")
	files, err := LoadFilesFromFSWithOptions(vfs, DefaultLoadOptions())
	require.NoError(t, err)
	for _, name := range []string{
		"code/Main.scala", "code/Token.sol", "code/lib.ml", "code/lib.mli",
		"code/core.clj", "code/app.ex", "code/app.exs", "code/Page.cls",
		"code/Handler.apex", "code/Page.page-meta.xml",
		"code/init.lua",
	} {
		require.Contains(t, files, name)
	}
	require.NotContains(t, files, "binary.dat")
}

func TestNewRootFromFS(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("x.java", `password = "s3cretValue"`)
	root, err := NewRootFromFS(vfs)
	require.NoError(t, err)
	require.False(t, root.IsEmpty())
	vals, err := root.FileFilter("*", "regexp", nil, []string{`(?i)password\s*=\s*["'][^"']+["']`})
	require.NoError(t, err)
	require.Greater(t, sfvm.ValuesLen(vals), 0)
}

func TestLoadFilesFromFSWithOptions_KeepsSourceScriptsInBin(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("bin/run.sh", `exec $foo`)
	vfs.AddFile("build/generated.go", `package generated`)

	files, err := LoadFilesFromFSWithOptions(vfs, DefaultLoadOptions())
	require.NoError(t, err)
	require.Contains(t, files, "bin/run.sh")
	require.NotContains(t, files, "build/generated.go")
}

func TestPatternRootMaterializesBoundedHitWindows(t *testing.T) {
	files := map[string]string{
		"many.txt": "key\n" + strings.Repeat("key\n", 5000),
	}
	root := NewRoot(files)
	all, err := root.FileFilter("many.txt", "regexp", nil, []string{`key`})
	require.NoError(t, err)
	require.Equal(t, 5001, sfvm.ValuesLen(all))

	root.SetSourceHitBatch(0, DefaultSourceHitBatchSize)
	first, err := root.FileFilter("many.txt", "regexp", nil, []string{`key`})
	require.NoError(t, err)
	require.Equal(t, DefaultSourceHitBatchSize, sfvm.ValuesLen(first))

	root.SetSourceHitBatch(DefaultSourceHitBatchSize, DefaultSourceHitBatchSize)
	second, err := root.FileFilter("many.txt", "regexp", nil, []string{`key`})
	require.NoError(t, err)
	require.Equal(t, DefaultSourceHitBatchSize, sfvm.ValuesLen(second))

	root.SetSourceHitBatch(5000, DefaultSourceHitBatchSize)
	last, err := root.FileFilter("many.txt", "regexp", nil, []string{`key`})
	require.NoError(t, err)
	require.Equal(t, 1, sfvm.ValuesLen(last))
	_, _, total := root.SourceHitBatch()
	require.Equal(t, 5001, total)
}

func TestMatchRegexpWithNegatives(t *testing.T) {
	files := map[string]string{
		"config.env": `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
OTHER=1
`,
		"example.env": `AWS_ACCESS_KEY_ID=AKIAEXAMPLEFAKEKEY
`,
		"ok.txt": "no secrets here",
	}
	// Positive matches both files; negative (?i)example|fake removes example.env.
	vals, err := MatchRegexpWithNegatives(files, "*", `\b(A3T[A-Z0-9]|AKIA|AGPA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}\b`, []string{`(?i)example|sample|test|fake`})
	require.NoError(t, err)
	require.Equal(t, 1, sfvm.ValuesLen(vals))
	first, _ := vals[0].(interface{ Path() string })
	require.Contains(t, first.Path(), "config.env")
}

func TestExecRule_PatternRegexNot(t *testing.T) {
	ruleContent := `
desc(
  mode: "source",
  language: "general",
  title: "aws key with negative",
  rule_id: "test-aws-neg",
  level: "critical",
)
${*}.pattern_regex_not(/\b(A3T[A-Z0-9]|AKIA|AGPA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}\b/, /(?i)example|sample|test|fake/) as $hit
alert $hit for {
  level: "critical",
  message: "aws key",
}
`
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(ruleContent)
	require.NoError(t, err)
	require.True(t, sfvm.FrameIsSourceMode(frame))

	files := map[string]string{
		"a.properties": `key=AKIAIOSFODNN7EXAMPLE`,
		"b.properties": `key=AKIAEXAMPLEFAKEKEY123`,
	}
	result, err := ExecFrame(frame, files)
	require.NoError(t, err)
	require.True(t, HasAlert(result))
	require.Equal(t, 1, AlertCount(result))
}

// unreadableSource keeps the path list but fails every body read. A cached
// completed search must not call it.
type unreadableSource struct{ paths []string }

func (s unreadableSource) Paths() []string { return s.paths }

func (s unreadableSource) Read(string) (string, error) {
	return "", fmt.Errorf("source body is no longer readable")
}

func TestMatchRegexp_CachesCompletedEmptySearch(t *testing.T) {
	root := NewRoot(map[string]string{"a.txt": "alpha"})
	_, err := root.FileFilter("*", "regexp", nil, []string{"absent"})
	require.ErrorContains(t, err, "no file contains data matching rule")

	hits, cached := root.SourceHits(sourceHitKey("*", []string{"absent"}))
	require.True(t, cached, "a completed zero-hit search must be cached")
	require.Empty(t, hits)

	// Repeat the same search with a source that cannot be read. Answering from
	// the cache repeats the original error; a rescan would return the read error.
	root.SetSource(unreadableSource{paths: []string{"a.txt"}})
	_, err = root.FileFilter("*", "regexp", nil, []string{"absent"})
	require.ErrorContains(t, err, "no file contains data matching rule")
}

func TestMatchRegexpWithNegatives_CachesCompletedEmptyResults(t *testing.T) {
	params := sfvm.SetFileFilterParam(nil, sfvm.FileFilterParamPatternNotList)

	// Empty positive: the old code returned before writing any cache entry, so
	// every execution window rescanned every file.
	root := NewRoot(map[string]string{"a.txt": "SECRET value"})
	_, err := root.FileFilter("*", "regexp", params, []string{"absent", "value"})
	require.ErrorContains(t, err, "no file contains data matching rule")
	root.SetSource(unreadableSource{paths: []string{"a.txt"}})
	_, err = root.FileFilter("*", "regexp", params, []string{"absent", "value"})
	require.ErrorContains(t, err, "no file contains data matching rule")

	// Fully filtered positive: cache the zero-kept negative result as well.
	root = NewRoot(map[string]string{"a.txt": "SECRET value"})
	_, err = root.FileFilter("*", "regexp", params, []string{"SECRET value", "value"})
	require.ErrorContains(t, err, "after negative filter")
	root.SetSource(unreadableSource{paths: []string{"a.txt"}})
	_, err = root.FileFilter("*", "regexp", params, []string{"SECRET value", "value"})
	require.ErrorContains(t, err, "after negative filter")
}

// TestMatchRegexpWithNegatives_ResultDoesNotShadowPlainSearch fixes why the
// plain search and the negative-filtered search keep separate cache keys: they
// are different result sets over the same positive expression.
func TestMatchRegexpWithNegatives_ResultDoesNotShadowPlainSearch(t *testing.T) {
	files := map[string]string{
		"a.txt": "alpha value",
		"b.txt": "alpha alpha",
	}
	params := sfvm.SetFileFilterParam(nil, sfvm.FileFilterParamPatternNotList)

	// Negative first: every hit overlaps the negative, so the filtered result
	// is empty. The plain search must still see all three hits.
	root := NewRoot(files)
	_, err := root.FileFilter("*", "regexp", params, []string{"alpha", "alpha"})
	require.ErrorContains(t, err, "after negative filter")
	vals, err := root.FileFilter("*", "regexp", nil, []string{"alpha"})
	require.NoError(t, err)
	require.Equal(t, 3, sfvm.ValuesLen(vals), "the filtered empty result must not shadow the plain search")

	// Plain first: the filter must still drop every hit instead of reusing the
	// unfiltered set as its result.
	root = NewRoot(files)
	vals, err = root.FileFilter("*", "regexp", nil, []string{"alpha"})
	require.NoError(t, err)
	require.Equal(t, 3, sfvm.ValuesLen(vals))
	_, err = root.FileFilter("*", "regexp", params, []string{"alpha", "alpha"})
	require.ErrorContains(t, err, "after negative filter")
}

// gatedSource blocks every read until the test releases the gate, so all
// concurrent callers are inside the cache-miss path before the first scan runs.
type gatedSource struct {
	paths  []string
	bodies map[string]string
	gate   chan struct{}
	reads  atomic.Int64
}

func (s *gatedSource) Paths() []string { return s.paths }

func (s *gatedSource) Read(p string) (string, error) {
	<-s.gate
	s.reads.Add(1)
	return s.bodies[p], nil
}

// runConcurrently releases every caller at once, keeps the gate closed while
// they reach the flight, then returns the total file reads and each error.
func runConcurrently(calls []func(root *sfvm.PatternRoot) error) (int64, []error) {
	paths := []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt"}
	bodies := make(map[string]string, len(paths))
	for _, p := range paths {
		bodies[p] = "alpha value"
	}
	src := &gatedSource{paths: paths, bodies: bodies, gate: make(chan struct{})}
	root := NewRoot(nil)
	root.SetSource(src)

	errs := make([]error, len(calls))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, call := range calls {
		wg.Add(1)
		go func(i int, call func(root *sfvm.PatternRoot) error) {
			defer wg.Done()
			<-start
			errs[i] = call(root)
		}(i, call)
	}
	close(start)
	// Every caller reaches the flight while the leader blocks in Read.
	time.Sleep(100 * time.Millisecond)
	close(src.gate)
	wg.Wait()
	return src.reads.Load(), errs
}

func repeated(n int, call func(root *sfvm.PatternRoot) error) []func(root *sfvm.PatternRoot) error {
	calls := make([]func(root *sfvm.PatternRoot) error, n)
	for i := range calls {
		calls[i] = call
	}
	return calls
}

func TestMatchRegexp_ConcurrentMissesShareOneScan(t *testing.T) {
	reads, errs := runConcurrently(repeated(16, func(root *sfvm.PatternRoot) error {
		_, err := root.FileFilter("*", "regexp", nil, []string{"absent"})
		// A zero-hit search is a completed result, not a failed one.
		require.ErrorContains(t, err, "no file contains data matching rule")
		return nil
	}))
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.EqualValues(t, 5, reads, "16 concurrent misses must scan the five files once")
}

func TestMatchRegexpWithNegatives_ConcurrentMissesShareOneScan(t *testing.T) {
	params := sfvm.SetFileFilterParam(nil, sfvm.FileFilterParamPatternNotList)
	reads, errs := runConcurrently(repeated(16, func(root *sfvm.PatternRoot) error {
		_, err := root.FileFilter("*", "regexp", params, []string{"alpha", "alpha"})
		// Every hit overlaps the negative, so the group ends in the filtered error.
		require.ErrorContains(t, err, "after negative filter")
		return nil
	}))
	for _, err := range errs {
		require.NoError(t, err)
	}
	// Five reads for the positive scan, five again when the filter re-reads the
	// files that hold a hit.
	require.EqualValues(t, 10, reads, "16 concurrent misses must run the positive scan and the filter once")
}

// TestMatchRegexp_PlainAndNegativeSharePositiveScan covers the nested flights:
// a negative search runs its positive scan inside the result flight, and a
// plain search on the same pattern must join that flight instead of scanning
// again.
func TestMatchRegexp_PlainAndNegativeSharePositiveScan(t *testing.T) {
	params := sfvm.SetFileFilterParam(nil, sfvm.FileFilterParamPatternNotList)
	calls := repeated(8, func(root *sfvm.PatternRoot) error {
		_, err := root.FileFilter("*", "regexp", params, []string{"alpha", "alpha"})
		require.ErrorContains(t, err, "after negative filter")
		return nil
	})
	calls = append(calls, func(root *sfvm.PatternRoot) error {
		_, err := root.FileFilter("*", "regexp", nil, []string{"alpha"})
		return err
	})
	reads, errs := runConcurrently(calls)
	for _, err := range errs {
		require.NoError(t, err)
	}
	// Five reads for the shared positive scan, five for the negative filter,
	// five for the plain search's editors.
	require.EqualValues(t, 15, reads,
		"eight negative and one plain caller must share one positive scan")
}
