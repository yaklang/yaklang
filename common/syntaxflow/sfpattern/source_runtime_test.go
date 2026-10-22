package sfpattern

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/minirehs"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func sourceFrames(t testing.TB, bodies ...string) []*sfvm.SFFrame {
	t.Helper()
	var frames []*sfvm.SFFrame
	for _, body := range bodies {
		frame, err := sfvm.CompileRule("desc(mode: source, language: general)\n" + body)
		require.NoError(t, err)
		frames = append(frames, frame)
	}
	return frames
}

func sourceAlertSnapshot(result *sfvm.SFFrameResult) []string {
	var out []string
	result.AlertSymbolTable.ForEach(func(name string, values sfvm.Values) bool {
		_ = values.Recursive(func(value sfvm.ValueOperator) error {
			hit := value.(*sfvm.SimpleValue)
			out = append(out, fmt.Sprintf("%s:%s:%d:%d:%s", name, hit.Path(), hit.Start(), hit.End(), hit.String()))
			return nil
		})
		return true
	})
	sort.Strings(out)
	return out
}

func TestSourceScanRuntime_DirectSearchesAndChainedBoundary(t *testing.T) {
	frames := sourceFrames(t,
		`${*.txt}.pattern_regex(/alpha/) as $a
${*.txt}.pattern_regex(/beta/) as $b
$a + $b as $hit
alert $hit`,
		`${*.go}.pattern_regex(/alpha/) as $hit
alert $hit`,
		`${*.txt}.pattern_regex(/alpha/) as $ctx
$ctx.regexp(/second/) as $hit
alert $hit`,
	)
	plan, err := NewSourceScanPlan(frames)
	require.NoError(t, err)
	defer plan.Close()
	require.Len(t, plan.patterns, 2, "same expression shared across rules/globs; chained search excluded")
	require.Len(t, plan.searches, 3)
	files := map[string]string{
		"dir/a.txt": "alpha beta second", "dir/b.go": "alpha", "ignored.bin": "alpha second",
	}
	runtime := plan.Start(context.Background(), files)
	defer runtime.Close()
	for _, frame := range frames {
		baseline, err := ExecFrame(frame, files)
		require.NoError(t, err)
		root := NewRoot(files)
		root.SetFileFilterMatcher(runtime.Matcher(context.Background()))
		actual, err := frame.Feed(sfvm.ValuesOf(root))
		require.NoError(t, err)
		require.Equal(t, sourceAlertSnapshot(baseline), sourceAlertSnapshot(actual))
	}
	stats := runtime.Stats()
	require.Equal(t, 2, stats.Files, "one shared scan per eligible file, not one per rule")
	require.Equal(t, int64(4), stats.Lookups)
}

func TestSourceScanRuntime_EmptyResultsAndFallback(t *testing.T) {
	frames := sourceFrames(t, `${*}.pattern_regex(/absent/) as $hit
alert $hit`)
	plan, err := NewSourceScanPlan(frames)
	require.NoError(t, err)
	defer plan.Close()
	files := map[string]string{"a.txt": "fallback"}
	runtime := plan.Start(context.Background(), files)
	defer runtime.Close()
	root := NewRoot(files)
	root.SetFileFilterMatcher(runtime.Matcher(context.Background()))
	for i := 0; i < 2; i++ {
		_, err := root.FileFilter("*", "regexp", nil, []string{"absent"})
		require.Error(t, err)
	}
	values, err := root.FileFilter("*", "regexp", nil, []string{"fallback"})
	require.NoError(t, err)
	require.Equal(t, 1, sfvm.ValuesLen(values))
	require.Equal(t, int64(2), runtime.Stats().Lookups, "empty cached searches must not fall through to a rescan")
}

func TestSourceScanRuntime_OffsetsAndPostprocessing(t *testing.T) {
	frames := sourceFrames(t,
		`${*}.pattern_regex(/(?<=prefix)key/) as $hit
alert $hit`,
		`${*}.pattern_regex(/(ab)\1/) as $hit
alert $hit`,
		`${*}.pattern_regex_not(/key/, /skip:key/) as $hit
alert $hit`,
		`${*}.pattern_regex(/key/) as $raw
${*}.pattern_regex(/skip:key/) as $ctx
$raw not_inside $ctx as $hit
alert $hit`,
		`${*}.pattern_regex(/absent/) as $a
${*}.pattern_regex(/key/) as $b
$a + $b as $hit
alert $hit`,
		`${*}.pattern_regex(/(?m)^key/) as $hit
alert $hit`,
		`${*}.pattern_regex(/^key/, /abab/) as $hit
alert $hit`,
		`${*}.pattern_regex(/key(?=suffix)/) as $hit
alert $hit`,
		`${*}.pattern_regex(/a|ab/) as $hit
alert $hit`,
		`${*}.pattern_regex(/^/) as $hit
alert $hit`,
	)
	files := map[string]string{
		"a.txt": "你好 prefixkey skip:key keysuffix abab\nkey\n",
		"b.txt": "skip:key", "c.txt": "prefixkey",
	}
	plan, err := NewSourceScanPlan(frames)
	require.NoError(t, err)
	defer plan.Close()
	runtime := plan.Start(context.Background(), files)
	defer runtime.Close()
	for i, frame := range frames {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			baseline, err := ExecFrame(frame, files)
			require.NoError(t, err)
			root := NewRoot(files)
			root.SetFileFilterMatcher(runtime.Matcher(context.Background()))
			actual, err := frame.Feed(sfvm.ValuesOf(root))
			require.NoError(t, err)
			require.Equal(t, sourceAlertSnapshot(baseline), sourceAlertSnapshot(actual))
		})
	}
}

func TestSourceScanRuntime_InvalidPatternDoesNotPoisonBatch(t *testing.T) {
	frames := sourceFrames(t, `${*}.pattern_regex(/key/) as $hit
alert $hit`)
	// Add a malformed regex directly to the compiled instructions; this also
	// covers legacy/corrupt opcode images that bypass the source parser.
	invalid := sourceFrames(t, `${*}.pattern_regex(/bad/) as $hit`)[0]
	for _, code := range invalid.Codes {
		if code.OpCode == sfvm.OpFileFilterReg {
			code.Values = []string{"["}
		}
	}
	plan, err := NewSourceScanPlan(append(frames, invalid))
	require.NoError(t, err)
	defer plan.Close()
	require.Len(t, plan.patterns, 1)
	runtime := plan.Start(context.Background(), map[string]string{"a.txt": "key"})
	defer runtime.Close()
	require.NoError(t, runtime.Wait(context.Background()))
	require.Equal(t, 1, runtime.Stats().Hits)
}

func TestSourceScanRuntime_WaitCancellationAndErrors(t *testing.T) {
	// A waiter timing out must not cancel the shared job or block on readiness.
	runtime := &SourceScanRuntime{done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, runtime.Wait(ctx), context.Canceled)
	close(runtime.done)
	require.NoError(t, runtime.Wait(context.Background()))

	frames := sourceFrames(t, `${*}.pattern_regex(/key/) as $hit`)
	plan, err := NewSourceScanPlan(frames)
	require.NoError(t, err)
	require.NoError(t, plan.Close())
	failed := plan.Start(context.Background(), map[string]string{"a.txt": "key"})
	defer failed.Close()
	require.ErrorIs(t, failed.Wait(context.Background()), minirehs.ErrClosed)
	root := NewRoot(failed.files)
	root.SetFileFilterMatcher(failed.Matcher(context.Background()))
	_, err = root.FileFilter("*", "regexp", nil, []string{"key"})
	require.True(t, errors.Is(err, minirehs.ErrClosed), "failed scan must never masquerade as an empty complete result")
}

func TestSourceScanRuntime_ConcurrentQueriesAndSnapshots(t *testing.T) {
	frames := sourceFrames(t, `${*}.pattern_regex(/key/) as $hit
alert $hit`)
	plan, err := NewSourceScanPlan(frames)
	require.NoError(t, err)
	defer plan.Close()
	leftFiles := map[string]string{"left.txt": "key key"}
	rightFiles := map[string]string{"right.txt": "你好key"}
	left := plan.Start(context.Background(), leftFiles)
	right := plan.Start(context.Background(), rightFiles)
	defer left.Close()
	defer right.Close()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			runtime, files, count := left, leftFiles, 2
			if i%2 != 0 {
				runtime, files, count = right, rightFiles, 1
			}
			root := NewRoot(files)
			root.SetFileFilterMatcher(runtime.Matcher(context.Background()))
			for n := 0; n < 4; n++ {
				values, err := root.FileFilter("*", "regexp", nil, []string{"key"})
				if err != nil || sfvm.ValuesLen(values) != count {
					t.Errorf("query %d: values=%d err=%v", i, sfvm.ValuesLen(values), err)
				}
			}
		}(i)
	}
	wg.Wait()
	require.Equal(t, 2, left.Stats().Hits)
	require.Equal(t, 1, right.Stats().Hits)
}

func TestSourceScanRuntime_BatchTotalAcrossDirectSearches(t *testing.T) {
	frames := sourceFrames(t, `${*}.pattern_regex(/key/) as $a
${*}.pattern_regex(/last/) as $b
$a + $b as $hit
alert $hit`)
	plan, err := NewSourceScanPlan(frames)
	require.NoError(t, err)
	defer plan.Close()
	files := map[string]string{"a.txt": strings.Repeat("key ", 1200) + "last"}
	runtime := plan.Start(context.Background(), files)
	defer runtime.Close()
	root := NewRoot(files)
	root.SetFileFilterMatcher(runtime.Matcher(context.Background()))
	for offset := 0; offset < 1200; offset += 512 {
		root.SetSourceHitBatch(offset, 512)
		_, err := frames[0].Feed(sfvm.ValuesOf(root))
		require.NoError(t, err)
		_, _, total := root.SourceHitBatch()
		require.Equal(t, 1200, total, "last, smaller search must not truncate the remaining windows")
	}
}

func BenchmarkSourceScanRuntime(b *testing.B) {
	var bodies []string
	for i := 0; i < 128; i++ {
		bodies = append(bodies, fmt.Sprintf("${*.txt}.pattern_regex(/needle%03d/) as $hit\nalert $hit", i))
	}
	frames := sourceFrames(b, bodies...)
	files := make(map[string]string)
	for i := 0; i < 64; i++ {
		files[fmt.Sprintf("file-%03d.txt", i)] = strings.Repeat("ordinary source text without a matching keyword\n", 256)
	}
	files["hit.txt"] = "needle000 needle064"
	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			for _, frame := range frames {
				if _, err := ExecFrame(frame, files); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("runtime", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			plan, err := NewSourceScanPlan(frames)
			if err != nil {
				b.Fatal(err)
			}
			runtime := plan.Start(context.Background(), files)
			for _, frame := range frames {
				root := NewRoot(files)
				root.SetFileFilterMatcher(runtime.Matcher(context.Background()))
				if _, err := frame.Feed(sfvm.ValuesOf(root)); err != nil {
					b.Fatal(err)
				}
			}
			runtime.Close()
			if err := plan.Close(); err != nil {
				b.Fatal(err)
			}
		}
	})
}
