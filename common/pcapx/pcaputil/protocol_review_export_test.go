package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The logged digest covers every public event value, including complete fields,
// metadata, raw bytes, timestamps and session snapshots. Run unchanged on both
// revisions to detect output drift; single-worker replay keeps arrival order.
func TestProtocolReviewFullOutput(t *testing.T) {
	total := 0
	for _, name := range append(append([]string{}, binReplayCorpus...), "session/http2-multiplex", "session/mysql-classic") {
		var wire []byte
		if strings.HasPrefix(name, "session/") {
			var err error
			wire, err = os.ReadFile(filepath.Join("testdata", "protocol-sessions", strings.TrimPrefix(name, "session/")+".pcap"))
			require.NoError(t, err)
		} else {
			wire = binCorpusBytes(t, name)
		}
		events, _, err := binReplay(t, wire, 1)
		if err != nil {
			require.True(t, name == "ndpi/ndpi-tls.pcap" || name == "ndpi/ndpi-http-connect.pcap", "%s: %v", name, err)
			require.True(t, strings.Contains(err.Error(), "unfilled sequence gap") || strings.Contains(err.Error(), "TCP SYN changes an established initial sequence number"), "%v", err)
		}
		data, err := json.Marshal(events)
		require.NoError(t, err)
		t.Logf("%s events=%d sha256=%x", name, len(events), sha256.Sum256(data))
		total += len(events)
	}
	require.Greater(t, total, 130)
}
