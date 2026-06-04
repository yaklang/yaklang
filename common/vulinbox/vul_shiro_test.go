package vulinbox

import (
	"math/rand"
	"testing"
)

func TestSelectRandomShiroKeySamplesFromFirstTen(t *testing.T) {
	limit := 10
	if len(keyList) < limit {
		limit = len(keyList)
	}

	allowed := make(map[string]struct{}, limit)
	for _, key := range keyList[:limit] {
		allowed[key] = struct{}{}
	}

	for seed := int64(0); seed < 1000; seed++ {
		key := selectRandomShiroKey(rand.New(rand.NewSource(seed)))
		if _, ok := allowed[key]; !ok {
			t.Fatalf("selected key outside first %d entries: seed=%d key=%s", limit, seed, key)
		}
	}
}
