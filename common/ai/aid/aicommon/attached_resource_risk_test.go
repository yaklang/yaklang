package aicommon

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAttachedRiskResourceSupportsMultipleIDs(t *testing.T) {
	resource, err := ParseAttachedResourceData(NewAttachedResource(
		AttachedResourceTypeRisk,
		"ids",
		`[3, 1, 3, 2]`,
	))
	require.NoError(t, err)

	risk, ok := resource.(*AttachedRiskResourceData)
	require.True(t, ok)
	require.Equal(t, []int64{3, 1, 2}, risk.IDs)
	require.Equal(t, AttachedResourceTypeRisk, risk.Type())
	rendered := risk.ToAttachData(nil)
	require.Contains(t, rendered, "Count: 3")
	require.Contains(t, rendered, "3, 1, 2")
}

func TestAttachedRiskResourceRejectsInvalidPayloads(t *testing.T) {
	for _, raw := range []string{
		"",
		"1",
		`"1"`,
		`{"ids":[1,2]}`,
		`[]`,
		`[0]`,
		`[-1]`,
		`[1.5]`,
		`["1"]`,
		`[null]`,
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := ParseAttachedResourceData(NewAttachedResource(AttachedResourceTypeRisk, "ids", raw))
			require.Error(t, err)
		})
	}
}

func TestAttachedRiskResourceDoesNotRegisterLegacyAliases(t *testing.T) {
	resource, err := ParseAttachedResourceData(NewAttachedResource("risk_id", "id", "1"))
	require.NoError(t, err)
	require.IsType(t, &DefaultAttachedResourceData{}, resource)
}

func TestAttachedRiskResourcePreviewBoundary(t *testing.T) {
	ids := make([]string, 0, AttachedRiskResourcePreviewIDs+1)
	for i := 100000; i <= 100000+AttachedRiskResourcePreviewIDs; i++ {
		ids = append(ids, fmt.Sprintf("%d", i))
	}
	preview, err := ParseAttachedResourceData(NewAttachedResource(
		AttachedResourceTypeRisk,
		"ids",
		"["+strings.Join(ids[:AttachedRiskResourcePreviewIDs], ",")+"]",
	))
	require.NoError(t, err)
	rendered := preview.ToAttachData(nil)
	require.Contains(t, rendered, "Count: 80")
	require.Contains(t, rendered, "100000")
	require.Contains(t, rendered, "100079")
	require.NotContains(t, rendered, "IDs are omitted")

	omitted, err := ParseAttachedResourceData(NewAttachedResource(
		AttachedResourceTypeRisk,
		"ids",
		"["+strings.Join(ids, ",")+"]",
	))
	require.NoError(t, err)
	rendered = omitted.ToAttachData(nil)
	require.Contains(t, rendered, "Count: 81")
	require.Contains(t, rendered, "IDs are omitted")
	require.NotContains(t, rendered, "100000")
	require.NotContains(t, rendered, "100080")
}

func TestAttachedRiskResourceLargeSelectionOmitsIDs(t *testing.T) {
	for _, count := range []int{2000, AttachedRiskResourceMaxIDs} {
		t.Run(fmt.Sprintf("%d", count), func(t *testing.T) {
			ids := make([]string, 0, count)
			for i := 1; i <= count; i++ {
				ids = append(ids, fmt.Sprintf("%d", i))
			}
			resource, err := ParseAttachedResourceData(NewAttachedResource(
				AttachedResourceTypeRisk,
				"ids",
				"["+strings.Join(ids, ",")+"]",
			))
			require.NoError(t, err)
			rendered := resource.ToAttachData(nil)
			require.Contains(t, rendered, fmt.Sprintf("Count: %d", count))
			require.Contains(t, rendered, "IDs are omitted")
		})
	}
}

func TestAttachedRiskSelectionScope(t *testing.T) {
	task := NewStatefulTaskBase("risk-scope", "review risks", context.Background(), nil, true)
	task.SetAttachedDatas([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeRisk, "ids", `[7,3,7]`),
		NewAttachedResource(AttachedResourceTypeRisk, "ids", `[5,3]`),
		NewAttachedResource(AttachedResourceTypeFile, "file_path", "/tmp/context.txt"),
	})
	required, ids := AttachedRiskSelectionScope(task)
	require.True(t, required)
	require.Equal(t, []int64{7, 3, 5}, ids)

	malformed := NewStatefulTaskBase("risk-scope-invalid", "review risks", context.Background(), nil, true)
	malformed.SetAttachedDatas([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeRisk, "ids", `not-json`),
	})
	required, ids = AttachedRiskSelectionScope(malformed)
	require.True(t, required, "malformed risk attachments must still impose a scope")
	require.Empty(t, ids)
}
