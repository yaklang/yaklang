//go:build hids && linux

package runtime

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

func TestFileTrackerIndexedExpiryBoundaryAndZeroTimestamp(t *testing.T) {
	t.Parallel()
	tracker := newFileTrackerWithConfig(shortTermContextConfig{window: time.Minute, maxFiles: 8})
	start := time.Unix(1700000000, 0)
	tracker.rememberFile(&model.File{Path: "/boundary", Mode: "0644"}, start)
	tracker.prune(start.Add(time.Minute))
	if _, exists := tracker.byPath["/boundary"]; !exists {
		t.Fatal("file expired at the inclusive boundary; original policy is strictly greater")
	}
	tracker.nextPrune = time.Time{}
	tracker.prune(start.Add(time.Minute + time.Nanosecond))
	if len(tracker.byPath) != 0 {
		t.Fatal("file did not expire after its window")
	}
	tracker.rememberFile(&model.File{Path: "/zero", Mode: "0644"}, time.Time{})
	tracker.nextPrune = time.Time{}
	tracker.prune(start.Add(24 * time.Hour))
	if _, exists := tracker.byPath["/zero"]; !exists {
		t.Fatal("zero-timestamp file lost its original non-expiring behavior")
	}
}

func TestFileTrackerIndexedEvictionUsesOutOfOrderLastSeen(t *testing.T) {
	t.Parallel()
	tracker := newFileTrackerWithConfig(shortTermContextConfig{window: time.Hour, maxFiles: 2})
	start := time.Unix(1700000000, 0)
	for _, item := range []struct {
		path   string
		second int
	}{{"/a", 10}, {"/b", 20}, {"/a", 5}, {"/c", 15}} {
		tracker.rememberFile(&model.File{Path: item.path, Mode: "0644"}, start.Add(time.Duration(item.second)*time.Second))
	}
	if _, exists := tracker.byPath["/a"]; exists {
		t.Fatal("out-of-order update did not move the eviction priority backwards")
	}
	if len(tracker.byPath) != 2 || tracker.seen.Len() != 2 {
		t.Fatal("capacity/index bound changed")
	}
	tracker.Apply(model.Event{Type: model.EventTypeFileChange, Timestamp: start.Add(21 * time.Second), File: &model.File{Path: "/b", Operation: "REMOVE"}})
	if _, exists := tracker.seen.byKey["/b"]; exists {
		t.Fatal("forgotten file left a live heap node")
	}
}

func TestDriftIndexedExpiryUsesFixedWindowAndInclusiveBoundary(t *testing.T) {
	t.Parallel()
	aggregator := newDriftAggregator(time.Minute, 8)
	start := time.Unix(1700000000, 0)
	aggregator.observe("a", start)
	if result := aggregator.observe("a", start.Add(59*time.Second)); result.count != 2 {
		t.Fatalf("unexpected count: %#v", result)
	}
	if result := aggregator.observe("a", start.Add(time.Minute)); result.count != 1 || !result.emit {
		t.Fatalf("lastSeen incorrectly extended the fixed window: %#v", result)
	}
}

func TestDriftIndexedEvictionAndThrottleKeepOutOfOrderSemantics(t *testing.T) {
	t.Parallel()
	aggregator := newDriftAggregator(time.Hour, 2)
	start := time.Unix(1700000000, 0)
	aggregator.observe("a", start.Add(10*time.Second))
	aggregator.observe("b", start.Add(20*time.Second))
	if result := aggregator.observe("a", start.Add(5*time.Second)); result.count != 2 || !result.emit || !result.lastSeenAt.Equal(start.Add(5*time.Second)) {
		t.Fatalf("out-of-order second hit changed: %#v", result)
	}
	aggregator.observe("c", start.Add(15*time.Second))
	if _, exists := aggregator.entries["a"]; exists {
		t.Fatal("eviction used window start instead of regressed lastSeen")
	}
	if result := aggregator.observe("b", start.Add(19*time.Second)); result.count != 2 || !result.emit {
		t.Fatalf("second hit must emit even backwards in time: %#v", result)
	}
	if result := aggregator.observe("b", start.Add(18*time.Second)); result.count != 3 || result.emit {
		t.Fatalf("backwards third hit bypassed throttle: %#v", result)
	}
}

func TestTrackerIndexesRemainBoundedUnderHotKeyUpdates(t *testing.T) {
	t.Parallel()
	tracker := newFileTrackerWithConfig(shortTermContextConfig{window: time.Hour, maxFiles: 8})
	aggregator := newDriftAggregator(time.Hour, 8)
	start := time.Unix(1700000000, 0)
	for index := 0; index < 20000; index++ {
		observed := start.Add(time.Duration(index%100) * time.Nanosecond)
		tracker.rememberFile(&model.File{Path: "/hot", Mode: "0644"}, observed)
		aggregator.observe("hot", observed)
	}
	for _, index := range []*observationTimeIndex{&tracker.seen, &aggregator.seen, &aggregator.windows} {
		if index.Len() != 1 || len(index.byKey) != 1 {
			t.Fatalf("hot key accumulated stale nodes: heap=%d map=%d", index.Len(), len(index.byKey))
		}
	}
	tracker.forget("/hot")
	aggregator.prune(start.Add(2 * time.Hour))
	if tracker.seen.Len() != 0 || aggregator.seen.Len() != 0 || aggregator.windows.Len() != 0 {
		t.Fatal("removed keys remained retained in an index")
	}
}

func scanFileTrackerPrune(entries map[string]trackedFile, observedAt time.Time, window time.Duration, maxEntries int, nextPrune *time.Time) {
	if len(entries) <= maxEntries && !nextPrune.IsZero() && observedAt.Before(*nextPrune) {
		return
	}
	for path, entry := range entries {
		if !entry.lastSeen.IsZero() && observedAt.Sub(entry.lastSeen) > window {
			delete(entries, path)
		}
	}
	for len(entries) > maxEntries {
		oldest := ""
		for path, entry := range entries {
			if oldest == "" || entry.lastSeen.Before(entries[oldest].lastSeen) {
				oldest = path
			}
		}
		delete(entries, oldest)
	}
	*nextPrune = observedAt.Add(shortTermContextPruneInterval)
}

func TestFileTrackerIndexMatchesScanPolicyForOutOfOrderEvents(t *testing.T) {
	t.Parallel()
	tracker := newFileTrackerWithConfig(shortTermContextConfig{window: time.Second, maxFiles: 8})
	expected := map[string]trackedFile{}
	var nextPrune time.Time
	random := rand.New(rand.NewSource(17))
	start := time.Unix(1700000000, 0)
	for step := 0; step < 3000; step++ {
		path := fmt.Sprintf("/cache/%d", random.Intn(16))
		observed := start.Add(time.Duration(random.Intn(2000))*time.Millisecond + time.Duration(step)*time.Nanosecond)
		switch random.Intn(10) {
		case 0:
			tracker.forget(path)
			delete(expected, path)
		case 1:
			tracker.prune(observed)
			scanFileTrackerPrune(expected, observed, tracker.window, tracker.maxEntries, &nextPrune)
		default:
			mode := fmt.Sprintf("mode-%d", step)
			tracker.rememberFile(&model.File{Path: path, Mode: mode}, observed)
			expected[path] = trackedFile{mode: mode, lastSeen: observed}
			scanFileTrackerPrune(expected, observed, tracker.window, tracker.maxEntries, &nextPrune)
		}
		if !reflect.DeepEqual(tracker.byPath, expected) || !tracker.nextPrune.Equal(nextPrune) {
			t.Fatalf("scan/index behavior differs at step %d", step)
		}
		if tracker.seen.Len() != len(expected) || len(tracker.seen.byKey) != len(expected) {
			t.Fatalf("file index leaked at step %d", step)
		}
	}
}

func scanDriftObserve(entries map[string]driftAggregateEntry, key string, observed time.Time, window time.Duration, capacity int) driftAggregationResult {
	for key, entry := range entries {
		if observed.Sub(entry.windowStartedAt) >= window {
			delete(entries, key)
		}
	}
	entry, exists := entries[key]
	if exists && observed.Sub(entry.windowStartedAt) < window {
		entry.count++
		entry.lastSeenAt = observed
		emit := entry.count == 2 || observed.Sub(entry.lastEmitAt) >= baselineDriftAggregationUpdateInterval
		if emit {
			entry.lastEmitAt = observed
		}
		entries[key] = entry
		return driftAggregationResult{key: key, windowStartedAt: entry.windowStartedAt, lastSeenAt: entry.lastSeenAt, count: entry.count, emit: emit}
	}
	entries[key] = driftAggregateEntry{windowStartedAt: observed, lastSeenAt: observed, lastEmitAt: observed, count: 1}
	for len(entries) > capacity {
		oldest := ""
		for key, entry := range entries {
			if oldest == "" || entry.lastSeenAt.Before(entries[oldest].lastSeenAt) {
				oldest = key
			}
		}
		delete(entries, oldest)
	}
	return driftAggregationResult{key: key, windowStartedAt: observed, lastSeenAt: observed, count: 1, emit: true}
}

func TestDriftIndexMatchesScanPolicyForOutOfOrderEvents(t *testing.T) {
	t.Parallel()
	aggregator := newDriftAggregator(time.Second, 8)
	expected := map[string]driftAggregateEntry{}
	random := rand.New(rand.NewSource(29))
	start := time.Unix(1700000000, 0)
	for step := 0; step < 3000; step++ {
		key := fmt.Sprintf("key-%d", random.Intn(16))
		observed := start.Add(time.Duration(random.Intn(2000))*time.Millisecond + time.Duration(step)*time.Nanosecond)
		actual := aggregator.observe(key, observed)
		want := scanDriftObserve(expected, key, observed, aggregator.window, aggregator.maxEntries)
		if actual != want || !reflect.DeepEqual(aggregator.entries, expected) {
			t.Fatalf("scan/index behavior differs at step %d: actual=%#v want=%#v", step, actual, want)
		}
		if aggregator.windows.Len() != len(expected) || aggregator.seen.Len() != len(expected) {
			t.Fatalf("drift index leaked at step %d", step)
		}
	}
}

func BenchmarkFileTrackerFullCacheChurn(b *testing.B) {
	for _, capacity := range []int{64, 4096} {
		b.Run(fmt.Sprintf("capacity_%d", capacity), func(b *testing.B) {
			tracker := newFileTrackerWithConfig(shortTermContextConfig{window: time.Hour, maxFiles: capacity})
			start := time.Unix(1700000000, 0)
			paths := make([]string, capacity+1)
			for index := range paths {
				paths[index] = fmt.Sprintf("/cache/file-%d", index)
				if index < capacity {
					tracker.rememberFile(&model.File{Path: paths[index], Mode: "0644"}, start.Add(time.Duration(index)*time.Nanosecond))
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				tracker.Apply(model.Event{Type: model.EventTypeFileChange, Timestamp: start.Add(time.Duration(capacity+index) * time.Nanosecond), File: &model.File{Path: paths[(capacity+index)%(capacity+1)], Mode: "0644"}})
			}
		})
	}
}

func BenchmarkDriftAggregatorSuppressedHits(b *testing.B) {
	for _, count := range []int{1, 16, 1024} {
		b.Run(fmt.Sprintf("keys_%d", count), func(b *testing.B) {
			aggregator := newDriftAggregator(time.Hour, 1024)
			start := time.Unix(1700000000, 0)
			for index := 0; index < count; index++ {
				aggregator.observe(fmt.Sprintf("key-%d", index), start)
			}
			aggregator.observe("key-0", start)
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				aggregator.observe("key-0", start)
			}
		})
	}
}
