package yakit

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func buildHookColorBenchmarkPackets(requestBodyBytes, responseBodyBytes int) ([]byte, []byte) {
	requestBody := bytes.Repeat([]byte("q"), requestBodyBytes)
	responseBody := bytes.Repeat([]byte("r"), responseBodyBytes)
	request := fmt.Appendf(nil,
		"POST /upload HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n",
		len(requestBody),
	)
	request = append(request, requestBody...)
	response := fmt.Appendf(nil,
		"HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n",
		len(responseBody),
	)
	response = append(response, responseBody...)
	return request, response
}

func TestHookColorWithoutRulesPreservesMatchedRuleMetadata(t *testing.T) {
	replacer := NewMITMReplacer()
	requestPacket, responsePacket := buildHookColorBenchmarkPackets(1024, 4096)
	request, err := http.NewRequest(http.MethodPost, "http://example.test/upload", bytes.NewReader(requestPacket))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	httpctx.AppendMatchedRule(request, &ypb.MITMContentReplacer{
		Color:    "red",
		ExtraTag: []string{"matched-rule"},
	})
	flow := &schema.HTTPFlow{}

	extracted := replacer.HookColor(requestPacket, responsePacket, request, flow)
	if len(extracted) != 0 {
		t.Fatalf("extracted data = %d, want 0", len(extracted))
	}
	if !flow.HasColor(schema.FLOW_COLOR_RED) {
		t.Fatalf("flow tags %q do not contain red", flow.Tags)
	}
	if !strings.Contains(flow.Tags, "matched-rule") {
		t.Fatalf("flow tags %q do not contain matched-rule", flow.Tags)
	}
}

func TestMITMReplacerHaveRulesRequiresEnabledRule(t *testing.T) {
	tests := []struct {
		name string
		rule *ypb.MITMContentReplacer
		want bool
	}{
		{name: "no records", want: false},
		{name: "blank rule", rule: &ypb.MITMContentReplacer{}, want: false},
		{
			name: "disabled rule",
			rule: &ypb.MITMContentReplacer{Rule: "needle", Disabled: true},
			want: false,
		},
		{name: "enabled rule", rule: &ypb.MITMContentReplacer{Rule: "needle"}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var replacer *MitmReplacer
			if test.rule == nil {
				replacer = NewMITMReplacer()
			} else {
				replacer = NewMITMReplacer(func() []*ypb.MITMContentReplacer {
					return []*ypb.MITMContentReplacer{test.rule}
				})
			}
			if got := replacer.HaveRules(); got != test.want {
				t.Fatalf("HaveRules() = %v, want %v", got, test.want)
			}
		})
	}
}

func BenchmarkHookColorWithoutRulesLargePackets(b *testing.B) {
	replacer := NewMITMReplacer()
	requestPacket, responsePacket := buildHookColorBenchmarkPackets(64*1024, 256*1024)
	request, err := http.NewRequest(http.MethodPost, "http://example.test/upload", bytes.NewReader(requestPacket))
	if err != nil {
		b.Fatalf("build request: %v", err)
	}
	flow := &schema.HTTPFlow{}
	b.ReportAllocs()
	b.SetBytes(int64(len(requestPacket) + len(responsePacket)))
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		replacer.HookColor(requestPacket, responsePacket, request, flow)
	}
}
