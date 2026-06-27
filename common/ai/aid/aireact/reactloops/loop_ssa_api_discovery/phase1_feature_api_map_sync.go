package loop_ssa_api_discovery

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

var featureApiMapPersistMu sync.Mutex

// mergeAndPersistHttpApiUnitResult merges one http api unit result into feature_api_map under a process-wide lock.
func mergeAndPersistHttpApiUnitResult(rt *Runtime, inv *FeatureInventoryV1, entry HttpApiUnitResult) error {
	if rt == nil {
		return utils.Error("nil runtime")
	}
	featureApiMapPersistMu.Lock()
	defer featureApiMapPersistMu.Unlock()

	apiMap, err := ensureFeatureApiMap(rt)
	if err != nil {
		return err
	}
	if inv != nil {
		mergeHttpApiUnitResultsIntoFeatureMap(inv, apiMap, []HttpApiUnitResult{entry})
	} else {
		mergeFeatureApiMapEntry(apiMap, FeatureApiMapEntry{
			FeatureID:   entry.FeatureID,
			Label:       entry.FeatureLabel,
			Processed:   true,
			Apis:        entry.Apis,
			ApiCount:    len(entry.Apis),
			NoApiReason: entry.NoApiReason,
		})
	}
	if err := persistFeatureApiMap(rt, apiMap); err != nil {
		return err
	}
	for _, feat := range apiMap.Features {
		if feat.FeatureID == entry.FeatureID {
			return syncFeatureApisToEndpoints(rt, feat)
		}
	}
	return nil
}
