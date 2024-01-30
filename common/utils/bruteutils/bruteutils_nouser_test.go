package bruteutils

import (
	"testing"
)

func TestNoPass(t *testing.T) {
	count := 0
	u, err := NewMultiTargetBruteUtilEx(
		WithOnlyNeedPassword(true),
		WithBruteCallback(func(item *BruteItem) *BruteItemResult {
			result := item.Result()
			result.Ok = true
			return result
		}),
		WithResultCallback(func(b *BruteItemResult) {
			count++
			t.Logf("result: %v", b)
		}),
		WithOkToStop(false),
	)
	if err != nil {
		t.FailNow()
	}
	u.Feed(&BruteItem{
		Target:   "host",
		Password: "abc",
	})
	u.Feed(&BruteItem{
		Target:   "host",
		Password: "abc",
	})
	u.Feed(&BruteItem{
		Target:   "host",
		Password: "abc",
	})
	u.Feed(&BruteItem{
		Target:   "host",
		Password: "abc",
	})
	u.Feed(&BruteItem{
		Target:   "host",
		Password: "123",
	})
	u.Run()
	if count != 2 {
		t.Fatal("count != 2")
	}
}
