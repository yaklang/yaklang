package linkprep

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// BuildManifest maps each canonical runtime symbol to a per-build link name.
func BuildManifest(seed []byte) (map[string]string, error) {
	if len(seed) < 16 {
		return nil, fmt.Errorf("linkprep: manifest seed too short (need >= 16 bytes)")
	}
	syms := CanonicalRuntimeSymbols()
	out := make(map[string]string, len(syms))
	for i, sym := range syms {
		h := sha256.Sum256(append(append(append([]byte{}, seed...), []byte(sym)...), byte(i), 0))
		out[sym] = "rt_" + hex.EncodeToString(h[:8])
	}
	return out, nil
}
