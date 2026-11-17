package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func displayNode(t *testing.T, nodes []DisplayNode, name string) DisplayNode {
	t.Helper()
	for _, node := range nodes {
		if node.Name == name {
			return node
		}
	}
	require.FailNow(t, "display node not found", name)
	return DisplayNode{}
}

func displayRangeText(raw []byte, ranges []DisplayRange) string {
	if len(ranges) == 0 || ranges[0].BitOffset%8 != 0 || ranges[0].BitLength%8 != 0 {
		return ""
	}
	start := ranges[0].BitOffset / 8
	end := start + ranges[0].BitLength/8
	if start < 0 || end < start || end > int64(len(raw)) {
		return ""
	}
	return string(raw[start:end])
}

func TestBuildDisplayFieldTreeHTTPRanges(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\nContent-Length: 11\r\nX-Repeat: one\r\nX-Repeat: two\r\n\r\nhello world")
	nodes, truncated := BuildHTTPDisplayNodes("http", raw, len(raw))
	require.False(t, truncated)
	require.Len(t, nodes, 1)
	http := nodes[0]
	line := displayNode(t, http.Children, "Response Line")
	require.Equal(t, "HTTP/1.1", displayRangeText(raw, displayNode(t, line.Children, "Protocol").Ranges))
	require.Equal(t, "200", displayRangeText(raw, displayNode(t, line.Children, "Status Code").Ranges))
	require.Equal(t, "OK", displayRangeText(raw, displayNode(t, line.Children, "Reason Phrase").Ranges))
	headers := displayNode(t, http.Children, "Headers")
	var repeats []string
	for _, header := range headers.Children {
		if header.Name == "X-Repeat" {
			repeats = append(repeats, displayRangeText(raw, header.Ranges))
		}
	}
	require.Equal(t, []string{"one", "two"}, repeats, "duplicate headers must retain separate wire spans")
	body := displayNode(t, http.Children, "Body (wire)")
	require.Equal(t, "hello world", displayRangeText(raw, body.Ranges))
}

func TestBuildDisplayFieldTreeHTTPRequestRanges(t *testing.T) {
	raw := []byte("POST /submit?q=1 HTTP/1.1\r\nHost: example.test\r\n\r\nbody")
	nodes, _ := BuildHTTPDisplayNodes("http", raw, len(raw))
	line := displayNode(t, nodes[0].Children, "Request Line")
	require.Equal(t, "POST", displayRangeText(raw, displayNode(t, line.Children, "Method").Ranges))
	require.Equal(t, "/submit?q=1", displayRangeText(raw, displayNode(t, line.Children, "Request URI").Ranges))
	require.Equal(t, "HTTP/1.1", displayRangeText(raw, displayNode(t, line.Children, "Protocol").Ranges))
	headers := displayNode(t, nodes[0].Children, "Headers")
	require.Equal(t, "example.test", displayRangeText(raw, displayNode(t, headers.Children, "Host").Ranges))
}

func TestBuildDisplayFieldTreeHTTPChunkedRanges(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5;ext=1\r\nhello\r\n6\r\n world\r\n0\r\n\r\n")
	nodes, _ := BuildHTTPDisplayNodes("http", raw, len(raw))
	first := displayNode(t, nodes[0].Children, "Chunk [0]")
	second := displayNode(t, nodes[0].Children, "Chunk [1]")
	require.Equal(t, "5", displayRangeText(raw, displayNode(t, first.Children, "Size").Ranges))
	require.Equal(t, "hello", displayRangeText(raw, displayNode(t, first.Children, "Content").Ranges))
	require.Equal(t, "6", displayRangeText(raw, displayNode(t, second.Children, "Size").Ranges))
	require.Equal(t, " world", displayRangeText(raw, displayNode(t, second.Children, "Content").Ranges))
}

func TestBuildDisplayFieldTreeDoesNotPersistHTTPPrefixRanges(t *testing.T) {
	tree := BuildDisplayFieldTree(&ProtocolEvent{Protocol: "http", Raw: []byte("GET / HTTP/1.1\r\n\r\n")})
	require.Empty(t, tree.Nodes)
}
