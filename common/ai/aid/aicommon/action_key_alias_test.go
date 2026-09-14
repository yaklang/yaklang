package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestActionKeyAlias(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
	}{
		{"canonical", `{"@action":"batch","calls":[{"params":{"type":"A","action":"lookup"}}]}`},
		{"alias", `{"action":"batch","calls":[{"params":{"type":"A","action":"lookup"}}]}`},
		{"alias_after_params", `{"calls":[{"params":{"type":"A","action":"lookup"}}],"action":"batch"}`},
		{"nested_canonical_before_alias", `{"calls":[{"params":{"type":"A","action":"lookup","@action":"other"}}],"action":"batch"}`},
		{"nested_canonical_after_alias", `{"action":"batch","calls":[{"params":{"type":"A","action":"lookup","@action":"other"}}]}`},
		{"canonical_after_alias", `{"action":"other","calls":[{"params":{"type":"A","action":"lookup"}}],"@action":"batch"}`},
		{"canonical_before_alias", `{"@action":"batch","action":"other","calls":[{"params":{"type":"A","action":"lookup"}}]}`},
		{"schema_alias", `{"type":"object","properties":{"action":{"const":"batch"},"calls":[{"params":{"type":"A","action":"lookup"}}]}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			action, err := ExtractAction(test.raw, "batch", "other")
			require.NoError(t, err)
			require.Equal(t, "batch", action.ActionType())
			require.Equal(t, "batch", action.Name())
			require.Equal(t, "batch", action.ObservedActionType())
			require.Equal(t, "batch", action.GetParams().GetString(ActionMagicKey))
			calls, exists, err := action.GetCanonicalObjectArray("calls")
			require.NoError(t, err)
			require.True(t, exists)
			require.Len(t, calls, 1)
			require.Equal(t, "A", calls[0].GetObject("params").GetString("type"))
			require.Equal(t, "lookup", calls[0].GetObject("params").GetString("action"))
		})
	}
}

func TestActionKeyAliasRejectsInvalidAndNestedMarkers(t *testing.T) {
	for _, test := range []struct {
		name     string
		raw      string
		observed string
	}{
		{"unsupported_alias", `{"action":"unsupported","params":{"type":"A"}}`, "unsupported"},
		{"unsupported_alias_with_nested_canonical", `{"params":{"@action":"batch"},"action":"unsupported"}`, "unsupported"},
		{"unsupported_canonical_after_alias", `{"action":"batch","@action":"unsupported"}`, "unsupported"},
		{"unsupported_canonical_before_alias", `{"@action":"unsupported","action":"batch"}`, "unsupported"},
		{"empty_canonical", `{"action":"batch","@action":""}`, ""},
		{"null_canonical", `{"action":"batch","@action":null}`, ""},
		{"invalid_alias", `{"action":["batch"]}`, ""},
		{"nested_alias", `{"params":{"action":"batch"}}`, ""},
		{"nested_canonical", `{"params":{"@action":"batch"}}`, ""},
		{"missing_marker", `{"tool":"read_file","params":{"path":"/config"}}`, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			action, err := ExtractActionFromStream(context.Background(), strings.NewReader(test.raw), "batch")
			require.NoError(t, err)
			require.NoError(t, action.WaitParseResult(context.Background()))
			require.Empty(t, action.ActionType())
			require.Equal(t, test.observed, action.ObservedActionType())
			_, err = ExtractAction(test.raw, "batch")
			require.Error(t, err)
		})
	}
}

func TestActionKeyAliasRejectsTruncatedObject(t *testing.T) {
	for _, key := range []string{"@action", "action"} {
		t.Run(key, func(t *testing.T) {
			raw := `{"` + key + `":"batch","calls":[{"params":{"type":"A"}},`
			action, err := ExtractActionFromStream(context.Background(), strings.NewReader(raw), "batch")
			require.NoError(t, err)
			require.ErrorContains(t, action.WaitParseResult(context.Background()), "complete canonical object")
			_, exists := action.LookupCanonicalParam("calls")
			require.False(t, exists)
			_, err = ExtractValidActionFromStream(context.Background(), strings.NewReader(raw), "batch")
			require.Error(t, err)
		})
	}
}

func TestExtractAllActionKeyAlias(t *testing.T) {
	actions := ExtractAllAction(`{"action":"first","params":{"action":"business"}} {"@action":"second","action":"ignored"}`)
	require.Len(t, actions, 2)
	require.Equal(t, "first", actions[0].ActionType())
	require.Equal(t, "business", actions[0].GetInvokeParams("params").GetString("action"))
	require.Equal(t, "second", actions[1].ActionType())
}
