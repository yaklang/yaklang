package bin_parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Assessment-only, known-profile replay. Captured packets and explicit bounded
// application PDUs are loaded before timing. This is NOT TCP reassembly,
// automatic classification, live capture, an RPC or a client implementation.
type currentReplayWork struct {
	packet currentCorpusWork
	app    *currentCorpusWork
	output int
}

func currentReplayWorks(tb testing.TB) []currentReplayWork {
	tb.Helper()
	apps := currentCorpusWorks(tb, true)
	byID := map[string]*currentCorpusWork{}
	for i := range apps {
		byID[apps[i].id] = &apps[i]
	}
	var result []currentReplayWork
	var matched int
	for _, packet := range currentCorpusWorks(tb, false) {
		capture, _, _ := strings.Cut(packet.id, "/")
		switch capture {
		case "ndpi-cassandra", "ndpi-memcached", "gen-memcache-bin", "pr5023-gen-memcache-bin":
		default:
			continue
		}
		w := currentReplayWork{packet: packet, app: byID[packet.id]}
		if w.app != nil {
			matched++
			if !bytes.HasSuffix(w.packet.wire, w.app.wire) {
				tb.Fatalf("application boundary changed: %s", packet.id)
			}
		}
		var err error
		w.output, err = currentReplayParse(w)
		if err != nil {
			tb.Fatal(err)
		}
		result = append(result, w)
	}
	if len(result) != 38 || matched != 11 {
		tb.Fatalf("replay inventory changed: %d packets / %d applications", len(result), matched)
	}
	return result
}

func currentReplayParse(w currentReplayWork) (int, error) {
	count, err := currentCorpusParse(w.packet, true)
	if err != nil || w.app == nil {
		return count, err
	}
	appCount, err := currentCorpusParse(*w.app, true)
	return count + appCount, err
}

type currentReplayEvent struct {
	work int
	due  time.Duration
	size int
}

// Due times follow cumulative CAPTURED packet bytes, not packet count. Ceiling
// division makes a packet available only once its full byte budget has arrived.
// Bounded configuration prevents arithmetic overflow and runaway test memory.
func currentReplayPlan(sizes []int, bps int64, window time.Duration) ([]currentReplayEvent, error) {
	if len(sizes) == 0 || bps < 1 || bps > 64_000_000 || window <= 0 || window > 30*time.Second {
		return nil, fmt.Errorf("invalid replay configuration")
	}
	for _, n := range sizes {
		if n <= 0 || n > 16*1024*1024 {
			return nil, fmt.Errorf("invalid packet length %d", n)
		}
	}
	budget := bps * int64(window) / int64(time.Second)
	var bits int64
	var result []currentReplayEvent
	for i := 0; ; i++ {
		next := bits + int64(sizes[i%len(sizes)])*8
		if next > budget {
			return result, nil
		}
		if len(result) == 1_000_000 {
			return nil, fmt.Errorf("replay exceeds one million events")
		}
		bits = next
		due := time.Duration((bits*int64(time.Second) + bps - 1) / bps)
		result = append(result, currentReplayEvent{i % len(sizes), due, sizes[i%len(sizes)]})
	}
}

type currentReplayObservation struct {
	queue, service, total time.Duration
	work, size, output    int
	finished              time.Duration
	err                   error
}

type currentReplayJob struct {
	event    currentReplayEvent
	intended time.Time
	enqueued time.Time
}

func currentReplayPercentiles(values []time.Duration) map[string]float64 {
	if len(values) == 0 {
		return nil // unavailable is not a measured zero
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	r := make(map[string]float64)
	for _, p := range []int{50, 95, 99, 100} {
		r[fmt.Sprintf("p%d_ms", p)] = float64(values[(len(values)*p+99)/100-1]) / 1e6
	}
	return r
}

// Arrivals never wait for available queue space or completed parsing. Slow
// producers catch up against the original schedule; that lateness is measured
// for ALL offers, including drops, and included in accepted-job total latency.
func currentReplayRun(plan []currentReplayEvent, workers, capacity int, window time.Duration, parse func(int) (int, error)) map[string]any {
	jobs := make(chan currentReplayJob, capacity)
	observations := make([][]currentReplayObservation, workers)
	var ready, done sync.WaitGroup
	ready.Add(workers)
	done.Add(workers)
	var started time.Time
	startWorkers := make(chan struct{})
	for worker := 0; worker < workers; worker++ {
		go func(worker int) {
			defer done.Done()
			ready.Done()
			<-startWorkers
			local := make([]currentReplayObservation, 0, len(plan)/workers+1)
			for job := range jobs {
				begin := time.Now()
				n, err := parse(job.event.work)
				end := time.Now()
				local = append(local, currentReplayObservation{
					queue: begin.Sub(job.enqueued), work: job.event.work,
					service: end.Sub(begin), total: end.Sub(job.intended),
					size: job.event.size, output: n, finished: end.Sub(started), err: err,
				})
			}
			observations[worker] = local
		}(worker)
	}
	ready.Wait()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started = time.Now()
	close(startWorkers)
	var dispatch []time.Duration
	var offeredBytes, dropped, droppedBytes, observedQueueMax int
	byRecord := make(map[int]map[string]int)
	for _, event := range plan {
		if byRecord[event.work] == nil {
			byRecord[event.work] = map[string]int{"offered": 0, "dropped": 0, "completed": 0, "parse_errors": 0}
		}
		byRecord[event.work]["offered"]++
		intended := started.Add(event.due)
		if delay := time.Until(intended); delay > 0 {
			time.Sleep(delay)
		}
		now := time.Now()
		dispatch = append(dispatch, now.Sub(intended))
		offeredBytes += event.size
		select {
		case jobs <- currentReplayJob{event, intended, now}:
			if n := len(jobs); n > observedQueueMax {
				observedQueueMax = n // observation, not an exact concurrent high-water mark
			}
		default:
			dropped++
			droppedBytes += event.size
			byRecord[event.work]["dropped"]++
		}
	}
	if remaining := time.Until(started.Add(window)); remaining > 0 {
		time.Sleep(remaining)
	}
	producerSeconds := time.Since(started).Seconds()
	close(jobs)
	done.Wait()
	wall := time.Since(started)
	runtime.ReadMemStats(&after)
	var service, queue, total []time.Duration
	var completed, failed, completedBytes, outputBytes, inWindow, inWindowBytes int
	var firstError string
	for _, local := range observations {
		for _, o := range local {
			completed++
			byRecord[o.work]["completed"]++
			completedBytes += o.size
			outputBytes += o.output
			if o.err != nil {
				failed++
				byRecord[o.work]["parse_errors"]++
				if firstError == "" {
					firstError = o.err.Error()
				}
			}
			if o.finished <= window {
				inWindow++
				inWindowBytes += o.size
			}
			service, queue, total = append(service, o.service), append(queue, o.queue), append(total, o.total)
		}
	}
	return map[string]any{
		"window_seconds": window.Seconds(), "producer_seconds": producerSeconds, "drained_seconds": wall.Seconds(),
		"workers": workers, "queue_capacity": capacity, "observed_queue_max": observedQueueMax,
		"offered": len(plan), "offered_bytes": offeredBytes, "offered_mbps": float64(offeredBytes) * 8 / window.Seconds() / 1e6,
		"completed": completed, "completed_bytes": completedBytes, "completed_in_window": inWindow, "completed_in_window_bytes": inWindowBytes,
		"completed_in_window_mbps":  float64(inWindowBytes) * 8 / window.Seconds() / 1e6,
		"completed_with_drain_mbps": float64(completedBytes) * 8 / wall.Seconds() / 1e6,
		"dropped":                   dropped, "dropped_bytes": droppedBytes, "parse_errors": failed, "first_error": firstError,
		"json_output_bytes": outputBytes, "dispatch_lateness": currentReplayPercentiles(dispatch),
		"queue_latency": currentReplayPercentiles(queue), "service_latency": currentReplayPercentiles(service), "scheduled_total_latency": currentReplayPercentiles(total),
		"allocated_bytes": after.TotalAlloc - before.TotalAlloc, "allocations": after.Mallocs - before.Mallocs,
		"gc_count": after.NumGC - before.NumGC, "gc_pause_ns": after.PauseTotalNs - before.PauseTotalNs,
		"end_heap_bytes": after.HeapAlloc, "by_record_index": byRecord,
	}
}

func TestCurrentCorpusBoundedReplay(t *testing.T) {
	if os.Getenv("BINPARSER_EVALUATE") != "1" {
		t.Skip("manual bounded replay assessment")
	}
	mbps, err := strconv.ParseInt(os.Getenv("BINPARSER_REPLAY_MBPS"), 10, 64)
	if err != nil || mbps < 1 || mbps > 64 {
		t.Fatal("set BINPARSER_REPLAY_MBPS to 1..64 captured-input Mbps")
	}
	window := 10 * time.Second
	if value := os.Getenv("BINPARSER_REPLAY_DURATION"); value != "" {
		window, err = time.ParseDuration(value)
		if err != nil {
			t.Fatal(err)
		}
	}
	works := currentReplayWorks(t)
	sizes := make([]int, len(works))
	var cycleBytes, cycleOutput int
	for i, work := range works {
		sizes[i] = len(work.packet.wire)
		cycleBytes += sizes[i]
		cycleOutput += work.output
	}
	plan, err := currentReplayPlan(sizes, mbps*1_000_000, window)
	if err != nil || len(plan) == 0 {
		t.Fatalf("invalid replay plan: %v", err)
	}
	result := currentReplayRun(plan, runtime.GOMAXPROCS(0), 64, window, func(i int) (int, error) {
		n, err := currentReplayParse(works[i])
		if err == nil && n != works[i].output {
			err = fmt.Errorf("JSON output length changed for %s", works[i].packet.id)
		}
		return n, err
	})
	result["requested_mbps"], result["cycle_packets"], result["cycle_applications"] = mbps, len(works), 11
	result["cycle_packet_bytes"], result["cycle_json_bytes"] = cycleBytes, cycleOutput
	result["workload"] = "known-profile-packet-and-application-json"
	byRecord := map[string]map[string]int{}
	for i, counts := range result["by_record_index"].(map[int]map[string]int) {
		if counts["completed"]+counts["dropped"] != counts["offered"] {
			t.Fatalf("record count conservation failed: %s", works[i].packet.id)
		}
		byRecord[works[i].packet.id] = counts
	}
	delete(result, "by_record_index")
	result["by_record"] = byRecord
	line, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("CURRENT_CORPUS_REPLAY %s", line)
	if result["completed"].(int)+result["dropped"].(int) != len(plan) || result["completed_bytes"].(int)+result["dropped_bytes"].(int) != result["offered_bytes"].(int) {
		t.Fatal("replay count or byte conservation failed")
	}
	if result["parse_errors"].(int) != 0 {
		t.Fatalf("replay parse errors: %s", result["first_error"])
	}
	// Queue drops and latency are findings, not correctness-test failures.
	// A passing Go test does not assert lossless capacity or a latency SLO.
}

func TestCurrentReplayPlanAndPercentiles(t *testing.T) {
	plan, err := currentReplayPlan([]int{100, 200}, 8000, time.Second)
	if err != nil || len(plan) != 7 {
		t.Fatalf("plan: %v / %v", plan, err)
	}
	want := []time.Duration{100, 300, 400, 600, 700, 900, 1000}
	for i, event := range plan {
		if event.due != want[i]*time.Millisecond || event.work != i%2 || event.size != []int{100, 200}[i%2] {
			t.Fatalf("event %d: %+v", i, event)
		}
	}
	for _, sizes := range [][]int{nil, {0}, {-1}, {16*1024*1024 + 1}} {
		if _, err := currentReplayPlan(sizes, 8000, time.Second); err == nil {
			t.Fatalf("accepted invalid sizes: %v", sizes)
		}
	}
	for _, bps := range []int64{0, -1, 64_000_001} {
		if _, err := currentReplayPlan([]int{1}, bps, time.Second); err == nil {
			t.Fatalf("accepted invalid rate: %d", bps)
		}
	}
	for _, window := range []time.Duration{0, -1, 30*time.Second + 1} {
		if _, err := currentReplayPlan([]int{1}, 8000, window); err == nil {
			t.Fatalf("accepted invalid window: %v", window)
		}
	}
	if _, err := currentReplayPlan([]int{1}, 64_000_000, 30*time.Second); err == nil {
		t.Fatal("event allocation must be bounded")
	}
	fractional, err := currentReplayPlan([]int{1}, 3, 3*time.Second)
	if err != nil || len(fractional) != 1 || fractional[0].due != 2666666667*time.Nanosecond {
		t.Fatalf("fractional rate rounded early: %v / %v", fractional, err)
	}
	if currentReplayPercentiles(nil) != nil {
		t.Fatal("missing data reported as zero")
	}
	p := currentReplayPercentiles([]time.Duration{3 * time.Millisecond, time.Millisecond, 2 * time.Millisecond})
	if p["p50_ms"] != 2 || p["p99_ms"] != 3 || p["p100_ms"] != 3 {
		t.Fatalf("nearest-rank percentiles: %v", p)
	}
}

func TestCurrentReplayAccounting(t *testing.T) {
	plan := []currentReplayEvent{{0, 0, 10}, {1, 0, 20}, {2, 0, 30}, {3, 0, 40}}
	r := currentReplayRun(plan, 2, 4, time.Millisecond, func(i int) (int, error) {
		if i == 2 {
			return 0, fmt.Errorf("intentional accounting error")
		}
		return i + 1, nil
	})
	if r["offered"] != 4 || r["completed"] != 4 || r["dropped"] != 0 || r["parse_errors"] != 1 || r["completed_bytes"] != 100 || r["json_output_bytes"] != 7 {
		t.Fatalf("accounting: %+v", r)
	}
	// Exercise the exact nonblocking queue admission contract independently of
	// thread scheduling. Full queues drop rather than reducing the offered rate.
	queue := make(chan currentReplayJob, 1)
	queue <- currentReplayJob{}
	select {
	case queue <- currentReplayJob{}:
		t.Fatal("full queue unexpectedly accepted work")
	default:
	}
}

func TestCurrentReplayWorkload(t *testing.T) {
	works := currentReplayWorks(t)
	var packetBytes, appBytes, outputBytes, applications int
	for _, work := range works {
		packetBytes += len(work.packet.wire)
		outputBytes += work.output
		if work.app != nil {
			applications++
			appBytes += len(work.app.wire)
		}
		if n, err := currentReplayParse(work); err != nil || n != work.output {
			t.Fatalf("nonrepeatable workload %s: %d / %v", work.packet.id, n, err)
		}
	}
	if len(works) != 38 || applications != 11 || packetBytes != 3897 || appBytes != 1421 || outputBytes != 33820 {
		t.Fatalf("workload changed: %d packets, %d apps, %d packet B, %d app B, %d JSON B", len(works), applications, packetBytes, appBytes, outputBytes)
	}
}
