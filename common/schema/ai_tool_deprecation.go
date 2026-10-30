package schema

import "fmt"

// DeprecatedAIToolNames reserves retired names across discovery, storage and
// invocation. Return a fresh slice so callers cannot mutate the policy.
func DeprecatedAIToolNames() []string {
	return []string{"analyze_pcap", "decrypt_pcap", "analyze_challenge_pcap"}
}

// AIToolDeprecationError also supplies migration instructions for old sessions
// and callers that still remember a retired tool name.
func AIToolDeprecationError(name string) error {
	switch name {
	case "analyze_pcap", "decrypt_pcap", "analyze_challenge_pcap":
		return fmt.Errorf("AI tool %q has been deprecated and removed (已废弃并移除). 请改用 pcapdb 流量分析系统: load_pcapfile with {\"pcapfile\":\"/absolute/path/capture.pcapng\"} imports the capture and returns dataset_id; query_pcapdb with {\"id\":\"<dataset_id>\",\"query_type\":\"metadata\"} inspects it, then use protocols/fields/sessions/streams/details for analysis; manage_pcapdb manages datasets. query_pcapdb also accepts pcapfile instead of id for automatic import. Do not retry the retired tool or translate its legacy parameters blindly", name)
	default:
		return nil
	}
}
