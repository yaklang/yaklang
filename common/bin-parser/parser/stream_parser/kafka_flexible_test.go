package stream_parser

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func flexFixture(t testing.TB, name string, v any) {
	t.Helper()
	b, e := os.ReadFile("../../../pcapx/pcaputil/testdata/protocol-sessions/kafka-flex/" + name)
	require.NoError(t, e)
	require.NoError(t, json.Unmarshal(b, v))
}
func flexHex(t testing.TB, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	require.NoError(t, e)
	return b
}
func flexRequest(api, ver int16, body []byte) []byte {
	w := make([]byte, 15)
	binary.BigEndian.PutUint16(w[4:], uint16(api))
	binary.BigEndian.PutUint16(w[6:], uint16(ver))
	w[12], w[13] = 255, 255
	w = append(w, body...)
	binary.BigEndian.PutUint32(w, uint32(len(w)-4))
	return w
}
func TestKafkaFlexibleRequestJavaOracle(t *testing.T) {
	var rows []struct {
		API, Version int16
		Request      string         `json:"request_hex"`
		Expected     map[string]any `json:"request"`
	}
	flexFixture(t, "java-oracle.json", &rows)
	for i, row := range rows {
		w := flexHex(t, row.Request)
		_, m, e := decodeKafkaFields(w, "request")
		require.NoError(t, e)
		require.Equal(t, int16(2), m["Header Version"])
		tags := m["Request Header"].(map[string]any)["Tagged Fields"].([]map[string]any)
		require.Equal(t, []byte{9, 8}, tags[0]["Data"])
		require.Equal(t, uint32(70), tags[0]["Tag"])
		tags = m["Tagged Fields"].([]map[string]any)
		require.Equal(t, []byte{7, 6, 5}, tags[0]["Data"])
		require.Equal(t, uint32(99), tags[0]["Tag"])
		switch row.API {
		case 18:
			require.Equal(t, row.Expected["clientSoftwareName"], m["Client Software Name"])
		case 3:
			require.Equal(t, row.Expected["topics"] == nil, m["All Topics"])
			if i == 1 {
				require.Empty(t, m["Topics"])
			}
			if i == 3 {
				require.Equal(t, []any{"zip-flex"}, m["Topics"])
			}
		case 0:
			require.EqualValues(t, row.Expected["acks"], m["Acks"])
			require.Nil(t, m["Transactional ID"])
			ps := m["Topic Results"].([]map[string]any)[0]["Partitions"].([]map[string]any)
			require.Len(t, ps, 2)
			for _, p := range ps {
				bs := p["Batches"].([]map[string]any)
				require.Len(t, bs, 1)
				require.True(t, bs[0]["CRC Valid"].(bool))
			}
		case 1:
			require.Equal(t, int32(-1), m["Replica ID"])
			ps := m["Topic Results"].([]map[string]any)[0]["Partitions"].([]map[string]any)
			require.Len(t, ps, 2)
			require.Equal(t, int32(-1), ps[0]["Last Fetched Epoch"])
		}
		// All shortened declared windows and trailing bytes must fail, independently of TCP framing.
		for cut := 15; cut < len(w); cut++ {
			bad := append([]byte(nil), w[:cut]...)
			binary.BigEndian.PutUint32(bad, uint32(len(bad)-4))
			_, _, err := decodeKafkaFields(bad, "request")
			require.Error(t, err, "api%d cut%d", row.API, cut)
		}
		bad := append(append([]byte(nil), w...), 0)
		binary.BigEndian.PutUint32(bad, uint32(len(bad)-4))
		_, _, e = decodeKafkaFields(bad, "request")
		require.Error(t, e)
	}
}
func TestKafkaFlexibleJavaTags(t *testing.T) {
	var rows []struct {
		Name         string
		API, Version int16
		Request      bool
		Body         string `json:"body_hex"`
	}
	flexFixture(t, "java-tags-oracle.json", &rows)
	require.Len(t, rows, 5)
	for _, row := range rows {
		t.Run(row.Name, func(t *testing.T) {
			b := flexHex(t, row.Body)
			var m map[string]any
			var e error
			if row.Request {
				_, m, e = decodeKafkaFields(flexRequest(row.API, row.Version, b), "request")
			} else {
				m, e = KafkaResponseBodyVersion(row.API, row.Version, b)
			}
			require.NoError(t, e)
			switch row.Name {
			case "fetch-cluster-tag":
				require.Equal(t, "cluster-tag", m["Cluster ID"])
			case "api-migration-tag":
				require.Equal(t, true, m["ZK Migration Ready"])
			case "produce-record-errors":
				p := m["Topic Results"].([]map[string]any)[0]["Partitions"].([]map[string]any)[0]
				require.Equal(t, int16(87), p["Error Code"])
				require.Equal(t, "rejected", p["Error Message"])
				es := p["Record Errors"].([]map[string]any)
				require.Len(t, es, 2)
				require.Nil(t, es[0]["Error Message"])
				require.Equal(t, "", es[1]["Error Message"])
			default:
				p := m["Topic Results"].([]map[string]any)[0]["Partitions"].([]map[string]any)[0]
				require.Equal(t, row.Name == "fetch-known-tags-null", p["Record Set Null"])
				require.Equal(t, row.Name == "fetch-known-tags-null", p["Aborted Transactions Null"])
				epoch := p["Diverging Epoch"].(map[string]any)
				require.Equal(t, int32(7), epoch["Epoch"])
				require.Equal(t, int64(123), epoch["End Offset"])
				require.Equal(t, []byte{4, 2}, epoch["Tagged Fields"].([]map[string]any)[0]["Data"])
				leader := p["Current Leader"].(map[string]any)
				require.Equal(t, int32(5), leader["Leader ID"])
				require.Equal(t, int32(9), leader["Leader Epoch"])
				snap := p["Snapshot ID"].(map[string]any)
				require.Equal(t, int64(120), snap["End Offset"])
				require.Equal(t, int32(6), snap["Epoch"])
			}
		})
	}
}
func TestKafkaFlexibleMalformed(t *testing.T) {
	for _, body := range [][]byte{
		{0, 1, 0},                    // required software name is null
		{255, 255, 255, 255, 16},     // uint32 overflow
		{128, 128, 128, 128, 128, 0}, // too many continuation bytes
		{1, 1, 1, 0, 2, 1},           // tag size exceeds window
		{1, 1, 2, 4, 0, 4, 0},        // duplicate tag
		{1, 1, 2, 5, 0, 4, 0},        // decreasing tag
		{1, 1, 129, 32},              // tag count budget
	} {
		_, _, e := decodeKafkaFields(flexRequest(18, 3, body), "request")
		require.Error(t, e, "%x", body)
	}
	// Known ZK migration tag must contain exactly one boolean, with a valid value.
	for _, tag := range [][]byte{{1, 3, 0}, {1, 3, 2, 1, 0}, {1, 3, 1, 2}} {
		body := append([]byte{0, 0, 1, 0, 0, 0, 0}, tag...)
		_, e := KafkaResponseBodyVersion(18, 3, body)
		require.Error(t, e)
	}
	for _, p := range [][2]int16{{0, 8}, {0, 10}, {1, 13}, {3, 10}, {18, 4}} {
		require.False(t, KafkaVersionSupported(p[0], p[1]))
		_, _, e := decodeKafkaFields(flexRequest(p[0], p[1], nil), "request")
		require.ErrorIs(t, e, ErrKafkaUnsupported)
	}
	// Empty header tags precede Metadata v9; an overflowing count cannot be a body.
	_, e := KafkaResponsePayloadVersion(3, 9, []byte{255, 255, 255, 255, 16})
	require.Error(t, e)
}
func FuzzKafkaFlexible(f *testing.F) {
	var rows []struct {
		API, Version int16
		Request      string  `json:"request_hex"`
		Response     *string `json:"response_hex"`
	}
	flexFixture(f, "java-oracle.json", &rows)
	for _, r := range rows {
		f.Add(byte(r.API), false, flexHex(f, r.Request))
		if r.Response != nil {
			f.Add(byte(r.API), true, flexHex(f, *r.Response)[8:])
		}
	}
	f.Fuzz(func(t *testing.T, api byte, response bool, w []byte) {
		if len(w) > 1<<20 {
			t.Skip()
		}
		versions := map[int16]int16{0: 9, 1: 12, 3: 9, 18: 3}
		a := int16(api)
		v, ok := versions[a]
		if !ok {
			return
		}
		if response {
			_, _ = KafkaResponsePayloadVersion(a, v, w)
		} else {
			_, _, _ = decodeKafkaFields(w, "request")
		}
	})
}
