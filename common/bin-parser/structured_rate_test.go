package bin_parser

import (
	"os"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
)

// Opt-in local capacity check, not NIC capture or TCP reassembly. The producer
// releases the unchanged corpus at a wall-clock byte rate; a single worker
// builds all fields and metadata. A bounded queue drops instead of hiding
// overload by blocking the producer. Leave ordinary regression tests fast.
func TestParseStructuredPaced100Mbps(t *testing.T) {
	testParseStructuredPaced(t, 100_000_000)
}

func TestParseStructuredPaced150Mbps(t *testing.T) {
	testParseStructuredPaced(t, 150_000_000)
}

func testParseStructuredPaced(t *testing.T, rate int64) {
	t.Helper()
	if os.Getenv("BIN_PARSER_RATE_TEST") != "1" {
		t.Skip("set BIN_PARSER_RATE_TEST=1 for the 20-second local rate check")
	}
	require.Equal(t, 1, runtime.GOMAXPROCS(0), "run with -test.cpu=1")
	works := currentCorpusWorks(t, true)
	bytesPerBatch := 0
	for _, w := range works {
		_, err := ParseStructured(w.wire, w.rule, w.entry)
		require.NoError(t, err)
		bytesPerBatch += len(w.wire)
	}
	const duration = 20 * time.Second
	// About 116 / 78 ms (1.46 MB) of logical input at 100 / 150 Mbps. The Windows timer
	// and desktop scheduler can deliver several intervals in one burst.
	const queueCapacity = 1024
	bitsPerBatch := int64(bytesPerBatch * 8)
	batchCount := int64(duration) * rate / (int64(time.Second) * bitsPerBatch)
	queue := make(chan time.Time, queueCapacity)
	type outcome struct {
		processed int
		delays    []time.Duration
		err       error
		value     map[string]any
	}
	finished := make(chan outcome, 1)
	proc, err := process.NewProcess(int32(os.Getpid()))
	require.NoError(t, err)
	cpuStart, err := proc.Times()
	require.NoError(t, err)
	start := time.Now()
	go func() {
		result := outcome{delays: make([]time.Duration, 0, int(batchCount))}
		for scheduled := range queue {
			for _, w := range works {
				value, err := ParseStructured(w.wire, w.rule, w.entry)
				if err != nil {
					result.err = err
					finished <- result
					return
				}
				result.value = value
			}
			result.processed++
			result.delays = append(result.delays, time.Since(scheduled))
			// A continuously nonempty channel need not yield on one CPU.
			// Bound each consumer turn so the paced producer can run on time.
			if result.processed%8 == 0 {
				runtime.Gosched()
			}
		}
		finished <- result
	}()
	var emitted, dropped int64
	maxQueued := 0
	lastWake, maxProducerGap := start, time.Duration(0)
	// Millisecond bursts model a paced source without a busy-wait producer.
	ticker := time.NewTicker(time.Millisecond)
	for emitted < batchCount {
		<-ticker.C
		now := time.Now()
		maxProducerGap = max(maxProducerGap, now.Sub(lastWake))
		lastWake = now
		due := min(batchCount, time.Since(start).Nanoseconds()*rate/(int64(time.Second)*bitsPerBatch))
		for emitted < due {
			scheduled := start.Add(time.Duration((emitted + 1) * bitsPerBatch * int64(time.Second) / rate))
			select {
			case queue <- scheduled:
			default:
				dropped++
			}
			emitted++
			maxQueued = max(maxQueued, len(queue))
		}
	}
	ticker.Stop()
	close(queue)
	result := <-finished
	elapsed := time.Since(start)
	cpuEnd, err := proc.Times()
	require.NoError(t, err)
	require.NoError(t, result.err)
	sort.Slice(result.delays, func(i, j int) bool { return result.delays[i] < result.delays[j] })
	require.NotEmpty(t, result.delays)
	throughput := float64(int64(result.processed)*bitsPerBatch) / elapsed.Seconds() / 1e6
	t.Logf("simulated input; workers=1 GOMAXPROCS=1 GOGC=%s target=%.0fMbps duration=%s batches=%d dropped=%d achieved=%.3fMbps cpu_seconds=%.6f max_queue=%d/%d producer_max_gap=%s batch_p99=%s batch_max=%s", os.Getenv("GOGC"), float64(rate)/1e6, elapsed, result.processed, dropped, throughput, cpuEnd.User+cpuEnd.System-cpuStart.User-cpuStart.System, maxQueued, queueCapacity, maxProducerGap, result.delays[len(result.delays)*99/100], result.delays[len(result.delays)-1])
	runtime.KeepAlive(result.value)
	require.Zero(t, dropped, "bounded simulated queue dropped input")
	require.EqualValues(t, batchCount, result.processed)
	require.GreaterOrEqual(t, throughput, float64(rate)/1e6*0.995, "allow timer granularity and final queue drain")
}
