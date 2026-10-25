package pcaputil

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlowCloseOrderDisambiguatesEqualEndpointHashesAcrossDomains(t *testing.T) {
	for seed := int64(0); seed < 24; seed++ {
		cache := &trafficFlowCache{items: map[flowKey]*flowEntry{}}
		for _, i := range rand.New(rand.NewSource(seed)).Perm(32) {
			key := flowKey{domain: CaptureDomain{Section: uint32(i / 8), Interface: i % 8}}
			cache.items[key] = &flowEntry{key: key, flow: &TrafficFlow{Hash: "same-endpoints", Index: uint64(i + 1)}}
		}
		var observed []uint64
		cache.ForEach(func(_ string, f *TrafficFlow) { observed = append(observed, f.Index) })
		for i, id := range observed {
			require.EqualValues(t, i+1, id, "every domain gets deterministic close callbacks and PDU allocation")
		}
	}
}
