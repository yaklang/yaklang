package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func snReadTypes(t *testing.T, value any) {
	t.Helper()
	switch v := value.(type) {
	case map[string]any:
		for k, x := range v {
			switch k {
			case "parameter":
				require.IsType(t, map[string]any{}, x, k)
			case "descriptors", "results", "elements", "rows":
				require.IsType(t, []map[string]any{}, x, k)
			case "wrapper_version", "source_wport", "destination_wport", "block_number", "block_number_ack", "class_id", "variable_name":
				require.IsType(t, uint16(0), x, k)
			case "command", "block_control", "window_size", "service_choice", "invoke_id_and_priority", "invoke_id", "unused_flag_bits_raw", "data_access_result", "access_selection_raw", "request_command", "type", "action_result", "return_choice_raw", "access_choice_raw", "result_choice_raw", "selector":
				require.IsType(t, byte(0), x, k)
			case "attribute_id", "method_id":
				require.IsType(t, int8(0), x, k)
			case "block_data_length", "list_count", "value_count", "parameter_count", "length", "contents_length":
				require.IsType(t, uint64(0), x, k)
			case "payload_length", "assembled_data_length", "Outstanding", "index", "apdu_offset", "encoded_length", "success_item_count", "error_item_count", "data_item_count", "row_count", "description_node_count", "value_node_count", "contents_offset":
				require.IsType(t, int(0), x, k)
			case "transfer_start_event", "acknowledged_block_event", "duplicate_of_event":
				require.IsType(t, uint64(0), x, k)
			}
			switch k {
			case "association_observed", "authentication_verified", "object_model_verified", "operation_success_verified", "selector_semantics_verified", "negative", "type_inferred_from_description":
				require.IsType(t, false, x, k)
			}
			if k == "value" && (v["type"] == byte(6) || v["type"] == byte(17) || v["type"] == byte(18)) {
				require.IsType(t, uint64(0), x, k)
			}
			snReadTypes(t, x)
		}
	case []map[string]any:
		for _, x := range v {
			snReadTypes(t, x)
		}
	case []any:
		for _, x := range v {
			snReadTypes(t, x)
		}
	}
}
