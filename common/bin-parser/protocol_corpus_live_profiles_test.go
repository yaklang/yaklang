package bin_parser

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Supplemental profiles are backed by complete TCP captures and public rule
// equivalence, not catalog-name exemptions. These deterministic fixtures are
// explicitly generated; the main HTTP/2/MySQL entries retain real nDPI evidence.
func requireLiveSupplementalProfile(t *testing.T, name, rule, entry, layer string) bool {
	t.Helper()
	type contract struct {
		file, rule, entry, key string
		value                  any
	}
	contracts := map[string]contract{
		"HTTP/2 Frame Sequence Fields":               {"http2-multiplex", "application-layer/http2_fields.yaml", "HTTP2FrameSequenceFields", "Frame Count", 2},
		"MySQL Auth Switch Fields":                   {"mysql-classic", "application-layer/mysql_fields.yaml", "MySQLAuthSwitchFields", "Plugin Name", "caching_sha2_password"},
		"MySQL Auth More Fields":                     {"mysql-classic", "application-layer/mysql_fields.yaml", "MySQLAuthMoreFields", "Auth Data", []byte{3}},
		"MySQL Auth Response Fields":                 {"mysql-classic", "application-layer/mysql_fields.yaml", "MySQLAuthResponseFields", "Sequence ID", uint64(3)},
		"MySQL Deprecated EOF Result Fields":         {"mysql-deprecated-eof", "application-layer/mysql_fields.yaml", "MySQLTextResultSetDeprecatedFields", "Row Count", 1},
		"MySQL Deprecated EOF Tracked Result Fields": {"mysql-tracked-eof", "application-layer/mysql_fields.yaml", "MySQLTextResultSetDeprecatedTrackFields", "Row Count", 1},
	}
	c, ok := contracts[name]
	if !ok {
		return false
	}
	require.Equal(t, c.rule, rule)
	require.Equal(t, c.entry, entry)
	require.Equal(t, "L7", layer)
	w, err := os.ReadFile(filepath.Join("..", "pcapx", "pcaputil", "testdata", "protocol-sessions", c.file+".pcap"))
	require.NoError(t, err)
	matched := false
	err = pcaputil.ReplayPcap(bytes.NewReader(w), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) {
		require.Equal(t, "decoded", e.Status, e.Error)
		if e.Entry != c.entry {
			return
		}
		metadata, ok := e.Metadata.(map[string]any)
		require.True(t, ok)
		if c.key == "Frame Count" && metadata[c.key] != c.value {
			return
		}
		require.Equal(t, c.value, metadata[c.key])
		matched = true
		node, err := parser.ParseBinary(&liveProfileBound{bytes.NewReader(e.Raw), uint64(len(e.Raw)) * 8}, e.Rule, e.Entry)
		require.NoError(t, err)
		require.Equal(t, e.Fields, stream_parser.NodeToMap(node))
		require.NotEmpty(t, e.Fields)
		_, err = parser.ParseStructured(e.Raw[:len(e.Raw)-1], e.Rule, e.Entry)
		require.Error(t, err)
	}))
	require.NoError(t, err)
	require.True(t, matched, "profile must be exercised by full capture")
	return true
}

type liveProfileBound struct {
	*bytes.Reader
	bits uint64
}

func (r *liveProfileBound) InputBitLength() uint64 { return r.bits }
