package aicommon

import "testing"

func TestShouldEmitConsumptionPayload(t *testing.T) {
	last := ""
	payload := map[string]any{
		"input_consumption":  int64(128007),
		"output_consumption": int64(8669),
		"consumption_uuid":   "consumption-1",
		"tier_consumption": map[string]any{
			"lightweight": map[string]any{
				"input_consumption":  int64(666),
				"output_consumption": int64(34),
			},
		},
	}

	if !shouldEmitConsumptionPayload(&last, payload) {
		t.Fatal("expected first payload emission")
	}
	if shouldEmitConsumptionPayload(&last, payload) {
		t.Fatal("expected identical payload to be suppressed")
	}

	changed := map[string]any{
		"input_consumption":  int64(128008),
		"output_consumption": int64(8669),
		"consumption_uuid":   "consumption-1",
		"tier_consumption": map[string]any{
			"lightweight": map[string]any{
				"input_consumption":  int64(667),
				"output_consumption": int64(34),
			},
		},
	}
	if !shouldEmitConsumptionPayload(&last, changed) {
		t.Fatal("expected changed payload emission")
	}
}
