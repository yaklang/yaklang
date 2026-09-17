package bin_parser

import (
	"bytes"
	"os"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
)

// Opt-in paced replay of the broad, explicitly framed corpus. Includes owned
// input copying, a bounded queue, one parser worker, all fields/metadata and GC.
// Excludes NIC capture, reassembly, first detection and downstream consumption.
// A nonblocking producer records overload; it never hides drops by waiting for
// the parser. This is a capacity experiment, not production drop policy.
func TestMessagePlanPaced150Mbps(t *testing.T) {
	if os.Getenv("BIN_PARSER_MESSAGE_PLAN_RATE") != "1" {
		t.Skip("opt-in 20-second capacity experiment")
	}
	require.Equal(t, 1, runtime.GOMAXPROCS(0))
	works := messagePlanCorpus(t)
	plans := make([]*StructuredPlan, len(works))
	ends := make([]int, len(works))
	var source []byte
	for i, w := range works {
		var err error
		plans[i], err = PrepareStructured(w.rule, w.entry)
		require.NoError(t, err)
		_, err = plans[i].Parse(w.wire)
		require.NoError(t, err)
		source = append(source, w.wire...)
		ends[i] = len(source)
	}
	const rate = int64(150_000_000)
	const duration = 20 * time.Second
	const capacity = 64
	bits := int64(len(source)) * 8
	count := int64(duration) * rate / (int64(time.Second) * bits)
	type job struct {
		scheduled time.Time
		wire      []byte
	}
	type outcome struct {
		processed int64
		delays    []time.Duration
		err       error
		last      map[string]any
	}
	queue := make(chan job, capacity)
	finished := make(chan outcome, 1)
	proc, err := process.NewProcess(int32(os.Getpid()))
	require.NoError(t, err)
	cpu, err := proc.Times()
	require.NoError(t, err)
	start := time.Now()
	go func() {
		out := outcome{delays: make([]time.Duration, 0, count)}
		for work := range queue {
			if out.err != nil {
				continue
			}
			at := 0
			for i, end := range ends {
				out.last, out.err = plans[i].Parse(work.wire[at:end:end])
				if out.err != nil {
					break
				}
				at = end
			}
			clear(work.wire) // queued inputs can be reclaimed/reused after parsing
			out.processed++
			out.delays = append(out.delays, time.Since(work.scheduled))
			runtime.Gosched()
		}
		finished <- out
	}()
	ticker := time.NewTicker(time.Millisecond)
	var emitted, dropped int64
	maxQueued := 0
	lastWake, maxGap := start, time.Duration(0)
	for emitted < count {
		<-ticker.C
		now := time.Now()
		maxGap = max(maxGap, now.Sub(lastWake))
		lastWake = now
		due := min(count, time.Since(start).Nanoseconds()*rate/(int64(time.Second)*bits))
		for emitted < due {
			scheduled := start.Add(time.Duration((emitted + 1) * bits * int64(time.Second) / rate))
			work := job{scheduled: scheduled, wire: bytes.Clone(source)}
			select {
			case queue <- work:
				maxQueued = max(maxQueued, len(queue))
			default:
				dropped++
			}
			emitted++
		}
	}
	ticker.Stop()
	queuedAtClose := len(queue)
	close(queue)
	out := <-finished
	elapsed := time.Since(start)
	endCPU, err := proc.Times()
	require.NoError(t, err)
	require.NoError(t, out.err)
	sort.Slice(out.delays, func(i, j int) bool { return out.delays[i] < out.delays[j] })
	percentile := func(p int) time.Duration {
		if len(out.delays) == 0 {
			return 0
		}
		return out.delays[(len(out.delays)-1)*p/100]
	}
	t.Logf("target=150 Mbps processed=%d/%d dropped=%d elapsed=%s delivered=%.3f Mbps cpu=%.3fs maxQueue=%d/%d queuedAtClose=%d delay p50=%s p95=%s p99=%s max=%s producerMaxGap=%s", out.processed, count, dropped, elapsed, float64(out.processed*bits)/elapsed.Seconds()/1e6, endCPU.User+endCPU.System-cpu.User-cpu.System, maxQueued, capacity, queuedAtClose, percentile(50), percentile(95), percentile(99), percentile(100), maxGap)
	require.Zero(t, dropped, "bounded queue overflow")
	require.Equal(t, count, out.processed)
	require.Less(t, elapsed, duration+500*time.Millisecond, "sustained backlog")
	runtime.KeepAlive(out.last)
}
