package diagnostics

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newTraceTestRecorder returns a recorder wired to an in-memory buffer (tests avoid log spam).
func newTraceTestRecorder() (*Recorder, *bytes.Buffer) {
	var buf bytes.Buffer
	rec := NewRecorder()
	rec.SetNested(true)
	rec.SetNestedLog(true, 0, &buf)
	return rec, &buf
}

func traceLines(buf string) []string {
	var out []string
	for _, line := range strings.Split(buf, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, traceHeader) {
			out = append(out, line)
		}
	}
	return out
}

func TestTraceLabNestedAndSteps(t *testing.T) {
	// outer has a nested inner lazybuild; inner is a leaf → single "|" line with path.
	rec, buf := newTraceTestRecorder()

	require.NoError(t, rec.TraceLab(NewLab(LabName("outer")), func() error {
		return rec.TraceLab(NewLab(LabName("inner"), LabDesc("a/b.go:1")), func() error {
			time.Sleep(5 * time.Millisecond)
			return nil
		})
	}))

	snap := rec.Snapshot()
	require.Len(t, snap, 2)

	lines := traceLines(buf.String())
	// Expected live log shape:
	//   TRACE \ outer
	//   TRACE     | inner  dur=...  a/b.go:1
	//   TRACE / outer  dur=...
	require.True(t, containsTraceWithAll(lines, `\ outer`), "missing outer begin:\n%s", strings.Join(lines, "\n"))
	require.True(t, containsTraceWithAll(lines, "| inner", "dur=", "a/b.go:1"))
	require.True(t, hasLineSuffix(lines, "/ outer", "dur="))
}

func TestTraceLabRecordsPanicWithoutReraise(t *testing.T) {
	rec := NewRecorder()
	rec.SetNested(true)
	rec.SetNestedLog(true, 0, io.Discard)
	err := rec.TraceLab(NewLab(LabName("panic-span")), func() error { panic("boom") })
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}

func TestTraceCompileLikeFormat(t *testing.T) {
	// Simulates compile: Track phase → nested TraceLab lazybuild tree → second Track step (leaf).
	rec, buf := newTraceTestRecorder()

	require.NoError(t, rec.Track("ParseProjectWithFS",
		func() error {
			return rec.TraceLab(NewLab(LabName("dvwaRedirect"), LabDesc("dvwa/includes/dvwaPage.inc.php:603")), func() error {
				time.Sleep(2 * time.Millisecond)
				return rec.TraceLab(NewLab(LabName("innerHelper"), LabDesc("dvwa/lib/helper.go:12")), func() error {
					time.Sleep(time.Millisecond)
					return nil
				})
			})
		},
		func() error {
			time.Sleep(time.Millisecond)
			return nil
		},
	))

	lines := traceLines(buf.String())
	// Paired \ / for nested spans; leaf steps collapse to TRACE | name  dur=...
	wantOrder := []string{
		`\ ParseProjectWithFS`,
		`\ dvwaRedirect`,
		`| innerHelper`,
		`/ dvwaRedirect`,
		`/ ParseProjectWithFS`,
		`| ParseProjectWithFS`,
	}
	require.True(t, subsequenceInOrder(lines, wantOrder), "trace order:\n%s", strings.Join(lines, "\n"))

	names := map[string]bool{}
	for _, m := range rec.Snapshot() {
		names[m.Name] = true
	}
	require.True(t, names["ParseProjectWithFS"])
	require.True(t, names["dvwaRedirect"])
	require.True(t, names["innerHelper"])
}

func TestTraceDuplicateNamesUsePath(t *testing.T) {
	// Same lazybuild name in two files must be distinguished by Desc path on the leaf line.
	rec, buf := newTraceTestRecorder()
	run := func(path string) error {
		return rec.TraceLab(NewLab(LabName("handler"), LabDesc(path)), func() error {
			time.Sleep(time.Millisecond)
			return nil
		})
	}
	require.NoError(t, run("pkg/a.go:10"))
	require.NoError(t, run("pkg/b.go:20"))

	lines := traceLines(buf.String())
	require.True(t, containsTraceWithAll(lines, "| handler", "pkg/a.go:10"))
	require.True(t, containsTraceWithAll(lines, "| handler", "pkg/b.go:20"))
	require.False(t, strings.Contains(buf.String(), `\ handler`), "duplicate-name leaves should not print begin")
}

func TestTraceTrackChildIndent(t *testing.T) {
	rec, buf := newTraceTestRecorder()
	require.NoError(t, rec.Track("ParseProjectWithFS", func() error {
		return rec.Track("ssa.Database.SaveIrIndexBatch", func() error {
			time.Sleep(time.Millisecond)
			return nil
		})
	}))
	lines := traceLines(buf.String())
	joined := strings.Join(lines, "\n")
	require.Contains(t, joined, `TRACE \ ParseProjectWithFS`)
	require.Contains(t, joined, `TRACE     | ssa.Database.SaveIrIndexBatch`)
	require.Contains(t, joined, `TRACE / ParseProjectWithFS`)
}

func TestTraceConcurrentSiblings(t *testing.T) {
	rec, buf := newTraceTestRecorder()
	require.NoError(t, rec.TraceLab(NewLab(LabName("parent")), func() error {
		var wg sync.WaitGroup
		wg.Add(2)
		worker := func(name string) {
			defer wg.Done()
			_ = rec.TraceLab(NewLab(LabName(name)), func() error {
				time.Sleep(3 * time.Millisecond)
				return nil
			})
		}
		go worker("child-a")
		go worker("child-b")
		wg.Wait()
		return nil
	}))

	out := buf.String()
	require.NotContains(t, out, `\ child-a`)
	require.NotContains(t, out, `\ child-b`)
	require.Contains(t, out, `  g=`)
	require.Contains(t, out, `TRACE \ parent`)
	require.Contains(t, out, `TRACE / parent  dur=`)
}

func TestTraceManyWorkersSameDepth(t *testing.T) {
	rec, buf := newTraceTestRecorder()
	const workers = 8
	require.NoError(t, rec.TraceLab(NewLab(LabName("holder")), func() error {
		var wg sync.WaitGroup
		wg.Add(workers)
		for i := 0; i < workers; i++ {
			i := i
			go func() {
				defer wg.Done()
				_ = rec.TraceLab(NewLab(LabName(fmt.Sprintf("worker-%d", i))), func() error {
					time.Sleep(time.Millisecond)
					return nil
				})
			}()
		}
		wg.Wait()
		return nil
	}))

	depths := map[int]int{}
	for _, line := range traceLines(buf.String()) {
		if strings.Contains(line, "worker-") && strings.Contains(line, "| worker-") {
			depths[traceLineIndentDepth(line)]++
		}
	}
	require.Len(t, depths, 1, "all workers share one indent depth:\n%s", buf.String())
}

func TestTrackAggregatesSubSteps(t *testing.T) {
	rec := NewRecorder()
	steps := []func() error{
		func() error { time.Sleep(time.Millisecond); return nil },
		func() error { time.Sleep(2 * time.Millisecond); return nil },
	}
	require.NoError(t, rec.Track("phase", steps...))
	snap := rec.Snapshot()
	require.Len(t, snap, 1)
	require.Equal(t, "phase", snap[0].Name)
	require.Equal(t, uint64(2), snap[0].Count)
}

func TestTrackDisabledAtHighLevel(t *testing.T) {
	orig := GetLevel()
	defer SetLevel(orig)
	SetLevel(LevelHigh)

	rec := NewRecorder()
	require.NoError(t, rec.Track("x", func() error { return nil }))
	require.Empty(t, rec.Steps())
}

func TestDefaultRecorderIsOptIn(t *testing.T) {
	rec := NewRecorder()
	require.False(t, rec.NestedEnabled(), "nested TRACE is off until explicitly enabled")
	applyDefaultOutput(rec)
	require.True(t, rec.NestedEnabled())
	_, isLog := rec.nestedWriter.(logLineWriter)
	require.True(t, isLog, "default trace sink should be project logger")
}

var traceIndentRE = regexp.MustCompile(`^TRACE( +)[\\/|]`)

func traceLineIndentDepth(line string) int {
	m := traceIndentRE.FindStringSubmatch(line)
	if m == nil {
		return -1
	}
	return len(m[1]) / len(traceIndentUnit)
}

func subsequenceInOrder(lines []string, markers []string) bool {
	mi := 0
	for _, line := range lines {
		if mi >= len(markers) {
			break
		}
		if strings.Contains(line, markers[mi]) {
			mi++
		}
	}
	return mi == len(markers)
}

func containsTraceWithAll(lines []string, parts ...string) bool {
	for _, line := range lines {
		ok := true
		for _, p := range parts {
			if !strings.Contains(line, p) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func hasLineSuffix(lines []string, namePart, suffix string) bool {
	for _, line := range lines {
		if strings.Contains(line, namePart) && strings.Contains(line, suffix) {
			return true
		}
	}
	return false
}
