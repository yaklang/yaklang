package bin_parser

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestProtocolCorpusContractReferencesExist closes the reverse side of the
// corpus ledger. The validation matrix proves that every manifest capture has
// one executable disposition; this test additionally proves that no removed or
// renamed capture can leave a stale test contract behind.
func TestProtocolCorpusContractReferencesExist(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"

	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "manifest.json"), &manifest)

	captures := make(map[string]protocolCorpusCapture, len(manifest.Captures))
	roadmapNames := make(map[string]struct{})
	for _, capture := range manifest.Captures {
		_, duplicate := captures[capture.ID]
		require.False(t, duplicate, "duplicate manifest capture id %q", capture.ID)
		captures[capture.ID] = capture
		if capture.RoadmapName != nil {
			roadmapNames[*capture.RoadmapName] = struct{}{}
		}
	}

	for _, reference := range protocolCorpusSortedContractReferences() {
		_, ok := captures[reference.captureID]
		require.Truef(t, ok, "%s contract points to missing capture %q", reference.kind, reference.captureID)
	}
	for name := range protocolCorpusParseContracts {
		_, ok := roadmapNames[name]
		require.Truef(t, ok, "parse contract points to missing roadmap name %q", name)
	}
	for name, spec := range protocolCorpusSupplementalProfiles {
		capture, ok := captures[spec.CaptureID]
		require.Truef(t, ok, "supplemental profile %q points to missing capture %q", name, spec.CaptureID)
		require.Greater(t, spec.Frame, 0)
		require.LessOrEqual(t, spec.Frame, capture.PacketCount)
	}

	catalog := make(map[string]ProtocolInfo, len(ProtocolCatalog))
	for _, info := range ProtocolCatalog {
		catalog[info.Name] = info
	}
	for name, spec := range protocolCorpusSupplementalProfiles {
		info, ok := catalog[name]
		require.Truef(t, ok, "supplemental profile %q has no catalog entry", name)
		require.Equal(t, spec.Contract.RuleFile, info.RuleFile)
		require.Equal(t, spec.Contract.EntryNode, info.EntryNode)
		require.Equal(t, spec.Contract.Layer, info.Layer)
	}
	validExactKeys := make(map[string]struct{})
	for _, capture := range manifest.Captures {
		if spec, ok := protocolCorpusCaptureParseSpecs[capture.ID]; ok {
			validExactKeys[capture.ID+"/"+spec.Name] = struct{}{}
			continue
		}
		if !protocolCorpusIsPositive(capture) || capture.RoadmapName == nil || capture.RepresentativeFrame == nil {
			continue
		}
		name := *capture.RoadmapName
		contract, hasContract := protocolCorpusParseContracts[name]
		_, inCatalog := catalog[name]
		if inCatalog || (hasContract && contract.RuleFile != "" && contract.Layer != "") {
			validExactKeys[capture.ID+"/"+name] = struct{}{}
		}
	}
	for _, spec := range protocolCorpusLayerParseSpecs {
		validExactKeys[spec.CaptureID+"/"+spec.Name] = struct{}{}
	}
	for key := range protocolCorpusExactValues {
		_, ok := validExactKeys[key]
		require.Truef(t, ok, "exact-value contract %q has no executable direct parse case", key)
	}
}

type protocolCorpusContractReference struct {
	kind      string
	captureID string
}

func protocolCorpusSortedContractReferences() []protocolCorpusContractReference {
	var references []protocolCorpusContractReference
	appendMap := func(kind string, ids []string) {
		for _, id := range ids {
			references = append(references, protocolCorpusContractReference{kind: kind, captureID: id})
		}
	}
	keys := func(values any) []string {
		var result []string
		switch values := values.(type) {
		case map[string]protocolCorpusCaptureParseSpec:
			for id := range values {
				result = append(result, id)
			}
		case map[string]protocolCorpusClassifierSpec:
			for id := range values {
				result = append(result, id)
			}
		case map[string]protocolCorpusRejectionSpec:
			for id := range values {
				result = append(result, id)
			}
		case map[string]string:
			for id := range values {
				result = append(result, id)
			}
		}
		sort.Strings(result)
		return result
	}
	appendMap("capture-parse", keys(protocolCorpusCaptureParseSpecs))
	appendMap("classifier", keys(protocolCorpusClassifierSpecs))
	appendMap("rejection", keys(protocolCorpusRejectionSpecs))
	appendMap("structural", keys(protocolCorpusStructuralSpecs))
	for _, spec := range protocolCorpusLayerParseSpecs {
		references = append(references, protocolCorpusContractReference{kind: "layer-parse", captureID: spec.CaptureID})
	}
	sort.Slice(references, func(i, j int) bool {
		left := references[i].kind + "\x00" + references[i].captureID
		right := references[j].kind + "\x00" + references[j].captureID
		return strings.Compare(left, right) < 0
	})
	return references
}
