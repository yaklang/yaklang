package sharkcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func http1MessageOrderBranch(t *testing.T, fields []field, name string) []field {
	t.Helper()
	for start, item := range fields {
		if item.text != name || !item.branch {
			continue
		}
		end := start + 1
		for end < len(fields) && fields[end].depth > item.depth {
			end++
		}
		return fields[start+1 : end]
	}
	t.Fatalf("missing %q branch", name)
	return nil
}

func http1MessageOrderIndex(t *testing.T, fields []field, prefix string) int {
	t.Helper()
	for index, item := range fields {
		if strings.HasPrefix(item.text, prefix) {
			return index
		}
	}
	t.Fatalf("missing %q", prefix)
	return -1
}

func TestHTTP1MessageFieldOrderAcrossViews(t *testing.T) {
	session, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test complete")
	request := []byte("POST /order HTTP/1.1\r\nHost: example.test\r\nContent-Length: 4\r\nX-Repeat: one\r\nX-Repeat: two\r\n\r\nDATA")
	response := []byte("HTTP/1.1 200 OK\r\nContent-Length: 4\r\nX-Repeat: one\r\nX-Repeat: two\r\n\r\nBODY")
	var events []*pcaputil.ProtocolEvent
	for direction, wire := range [][]byte{request, response} {
		result := session.Feed(direction, time.Unix(int64(100+direction), 0), wire)
		require.Nil(t, result.Err)
		require.Len(t, result.Events, 1)
		event := result.Events[0]
		event.Fields, err = event.GetFields()
		require.NoError(t, err)
		require.Equal(t, "decoded", event.Status)
		require.Equal(t, wire, event.Raw)
		events = append(events, event)
	}
	before, err := json.Marshal(events)
	require.NoError(t, err)
	fields, sources := protocolEventDetail(events)
	views := []struct {
		name   string
		detail packetDetail
	}{
		{"local", packetDetail{fields: fields, sources: sources}},
		{"packet-worker", workerMetadataDetail(t, events, false)},
		{"stream-worker", workerMetadataDetail(t, events, true)},
	}
	for _, view := range views {
		t.Run(view.name, func(t *testing.T) {
			for _, kind := range []string{"HTTP Request", "HTTP Response"} {
				message := http1MessageOrderBranch(t, view.detail.fields, kind)
				first := http1MessageOrderIndex(t, message, "FirstLine")
				header := http1MessageOrderIndex(t, message, "Headers[")
				body := http1MessageOrderIndex(t, message, "Body")
				require.Less(t, first, header, "the start line precedes header fields")
				require.Less(t, header, body, "headers precede the body")
				line := http1MessageOrderBranch(t, message, "FirstLine")
				ordered := []string{"Version: HTTP/1.1", "Status: 200", "Message: OK"}
				if kind == "HTTP Request" {
					ordered = []string{"Method: POST", "Path: /order", "Version: HTTP/1.1"}
				}
				for i := 1; i < len(ordered); i++ {
					require.Less(t, http1MessageOrderIndex(t, line, ordered[i-1]), http1MessageOrderIndex(t, line, ordered[i]))
				}
				require.Less(t, http1MessageOrderIndex(t, message, "Headers["+map[string]string{"HTTP Request": "2", "HTTP Response": "1"}[kind]+"]: X-Repeat: one"), http1MessageOrderIndex(t, message, "Headers["+map[string]string{"HTTP Request": "3", "HTTP Response": "2"}[kind]+"]: X-Repeat: two"))
			}
			for _, wire := range [][]byte{request, response} {
				matched := 0
				for _, source := range view.detail.sources {
					if bytes.Equal(source.Data, wire) {
						matched++
					}
				}
				require.Equal(t, 1, matched, "each exact message retains its own source")
			}
		})
	}
	after, err := json.Marshal(events)
	require.NoError(t, err)
	require.Equal(t, before, after, "display sorting must not rewrite SDK fields, raw bytes or session evidence")
}

func TestHTTP1StartLineOrderingDoesNotLeakIntoBodyOrOtherProtocol(t *testing.T) {
	for _, protocol := range []string{"http", "redis"} {
		t.Run(protocol, func(t *testing.T) {
			// These owned projection fixtures test scope, not protocol admission.
			line := map[string]any{"Version": "HTTP/1.1", "Status": 200, "Message": "OK"}
			bodyLine := map[string]any{"Version": "payload-version", "Status": 7, "Message": "payload-message"}
			event := &pcaputil.ProtocolEvent{ID: 1, Protocol: protocol, Status: "decoded", Raw: []byte("owned projection fixture"), Fields: map[string]any{"Message": map[string]any{"HTTP Response": map[string]any{"FirstLine": line, "Headers": []any{"X-Test: value"}, "Body": map[string]any{"FirstLine": bodyLine}, "Extra Observation": "retained"}}}}
			fields, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{event})
			message := http1MessageOrderBranch(t, fields, "HTTP Response")
			body := http1MessageOrderBranch(t, message, "Body")
			nested := http1MessageOrderBranch(t, body, "FirstLine")
			require.Equal(t, []string{"Status: 7", "Message: payload-message", "Version: payload-version"}, []string{nested[0].text, nested[1].text, nested[2].text}, "a payload dictionary retains its generic ordering")
			_ = http1MessageOrderIndex(t, message, "Extra Observation: retained")
			if protocol != "http" {
				require.Less(t, http1MessageOrderIndex(t, message, "Headers["), http1MessageOrderIndex(t, message, "Body"))
				require.Less(t, http1MessageOrderIndex(t, message, "Body"), http1MessageOrderIndex(t, message, "FirstLine"))
			}
		})
	}
}
