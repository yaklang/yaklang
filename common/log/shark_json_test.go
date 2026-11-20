package log

import "testing"

func TestIsSharkJSONCommand(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"yak"}, false},
		{[]string{"yak", "shark", "-r", "in.pcap"}, false},
		{[]string{"yak", "shark", "-r", "in.pcap", "--json"}, true},
		{[]string{"yak", "shark", "-r", "in.pcap", "--json-file", "out.jsonl"}, true},
		{[]string{"yak", "shark", "-r", "in.pcap", "--json-file=out.jsonl"}, true},
		{[]string{"yak", "shark", "-r", "in.pcap", "--json-physical-only"}, true},
	} {
		if got := isSharkJSONCommand(tc.args); got != tc.want {
			t.Fatalf("args %v: got %v want %v", tc.args, got, tc.want)
		}
	}
}
