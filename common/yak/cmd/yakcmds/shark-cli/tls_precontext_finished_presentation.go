package sharkcli

import "encoding/hex"

// Supplemental plaintext must carry a bounded, distinct original-PDU manifest.
// The regular Finished helper still reparses every body byte and checks its
// version, cipher, authentication and length before attaching any source.
func tlsPrecontextFinishedContext(values map[string]any, current uint64) bool {
	if values["Precontext Finished Recovery Verified"] != true || number(values["Negotiated Version"]) != 0x303 ||
		len(mapList(values["Recovered Context Handshake Messages"])) != 1 {
		return false
	}
	ids, limited := detailScalarSequence(values["Precontext Finished Original PDUs"], 4)
	items, ok := ids.([]any)
	if !ok || limited || len(items) < 1 || len(items) > 4 {
		return false
	}
	hashes, limited := detailScalarSequence(values["Precontext Finished Original Record SHA256"], 4)
	digests, ok := hashes.([]any)
	if !ok || limited || len(digests) != len(items) {
		return false
	}
	seen := map[uint64]bool{}
	for i, item := range items {
		id, ok := gpeHeaderInteger(item, ^uint64(0))
		if !ok || id == 0 || id == current || seen[id] {
			return false
		}
		seen[id] = true
		digest, ok := digests[i].(string)
		if !ok || len(digest) != 64 {
			return false
		}
		if raw, err := hex.DecodeString(digest); err != nil || len(raw) != 32 {
			return false
		}
	}
	return true
}
