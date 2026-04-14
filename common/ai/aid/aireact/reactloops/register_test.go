package reactloops

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestGetAllLoopMetadata_OrderIsStableAcrossCalls(t *testing.T) {
	uniquePrefix := fmt.Sprintf("__test_loop_metadata_order_%d__", time.Now().UnixNano())
	firstLoopName := uniquePrefix + "_first"
	secondLoopName := uniquePrefix + "_second"

	registerTestLoopWithMetadata := func(name, desc string) {
		t.Helper()
		err := RegisterLoopFactory(
			name,
			func(r aicommon.AIInvokeRuntime, opts ...ReActLoopOption) (*ReActLoop, error) {
				return nil, nil
			},
			WithLoopDescription(desc),
		)
		if err != nil {
			t.Fatalf("failed to register test loop %q: %v", name, err)
		}
	}

	registerTestLoopWithMetadata(firstLoopName, "first metadata order test loop")
	registerTestLoopWithMetadata(secondLoopName, "second metadata order test loop")

	for i := 0; i < 10; i++ {
		firstSnapshot := getLoopMetadataNames(GetAllLoopMetadata())
		secondSnapshot := getLoopMetadataNames(GetAllLoopMetadata())

		if !reflect.DeepEqual(firstSnapshot, secondSnapshot) {
			t.Fatalf("GetAllLoopMetadata order changed between calls:\nfirst:  %v\nsecond: %v", firstSnapshot, secondSnapshot)
		}

		firstIndex := indexOfLoopMetadata(firstSnapshot, firstLoopName)
		secondIndex := indexOfLoopMetadata(firstSnapshot, secondLoopName)
		if firstIndex == -1 || secondIndex == -1 {
			t.Fatalf("registered test loops not found in metadata snapshot: first=%d second=%d snapshot=%v", firstIndex, secondIndex, firstSnapshot)
		}
		if firstIndex >= secondIndex {
			t.Fatalf("expected registration order to be preserved, got %q at %d and %q at %d", firstLoopName, firstIndex, secondLoopName, secondIndex)
		}
	}
}

func getLoopMetadataNames(metadata []*LoopMetadata) []string {
	names := make([]string, 0, len(metadata))
	for _, meta := range metadata {
		if meta == nil {
			names = append(names, "")
			continue
		}
		names = append(names, meta.Name)
	}
	return names
}

func indexOfLoopMetadata(names []string, target string) int {
	for i, name := range names {
		if name == target {
			return i
		}
	}
	return -1
}
