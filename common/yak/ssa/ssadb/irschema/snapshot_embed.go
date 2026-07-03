package irschema

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed migrations/*.snapshot.json
var snapshotFS embed.FS

// expectedBaselineSnapshot loads the embedded baseline structural snapshot
// (migrations/0001_baseline.snapshot.json). It is the in-binary mirror of
// 0001_baseline.up.sql — captured once when the baseline was frozen, and
// verified by drift_test.go against a live PG16 instance.
//
// Returns (nil, nil) if no snapshot file is embedded (conservative:
// baselineMatchesActual then refuses to adopt without --force-adopt).
func expectedBaselineSnapshot() (*SchemaSnapshot, error) {
	data, err := snapshotFS.ReadFile("migrations/0001_baseline.snapshot.json")
	if err != nil {
		// Not all baseline versions carry a snapshot; treat as "unavailable".
		return nil, nil
	}
	var s SchemaSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("irschema: parse baseline snapshot: %w", err)
	}
	return &s, nil
}
