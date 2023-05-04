package arptable

import (
	"fmt"
	"testing"
	"time"
)

var (
	_ = fmt.Println
)

func TestTable(t *testing.T) {

	table := Table()
	if table == nil {
		t.Errorf("Empty table")
	}
}

func TestCacheInfo(t *testing.T) {
	prevUpdated := CacheLastUpdate().UnixNano()
	prevCount := CacheUpdateCount()

	CacheUpdate()

	if prevUpdated == CacheLastUpdate().UnixNano() {
		t.Error()
	}

	if prevCount == CacheUpdateCount() {
		t.Error()
	}
}

func TestAutoRefresh(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping autorefresh test")
	}
	prevUpdated := CacheLastUpdate().UnixNano()
	prevCount := CacheUpdateCount()

	AutoRefresh(100 * time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	StopAutoRefresh()

	if prevUpdated == CacheLastUpdate().UnixNano() {
		t.Error()
	}

	if prevCount == CacheUpdateCount() {
		t.Error()
	}

	// test to make sure stop worked
	prevUpdated = CacheLastUpdate().UnixNano()
	prevCount = CacheUpdateCount()
	time.Sleep(200 * time.Millisecond)
	if prevUpdated != CacheLastUpdate().UnixNano() {
		t.Error()
	}

	if prevCount != CacheUpdateCount() {
		t.Error()
	}
}

func TestSearch(t *testing.T) {
	table := Table()

	for ip, test := range table {

		result := Search(ip)
		if test != result {
			t.Errorf("expected %s got %s", test, result)
		}
	}
}

func BenchmarkSearch(b *testing.B) {
	table := Table()
	if len(table) == 0 {
		return
	}

	for ip, _ := range Table() {
		for i := 0; i < b.N; i++ {
			Search(ip)
		}

		// using the first key is enough
		break
	}
}
