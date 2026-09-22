package pcaputil

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"testing"
	"time"
)

func m2bPreparedSteps(deprecated, cursor bool) []sessionStep {
	steps := append([]sessionStep(nil), mysqlTestFixtures(deprecated)[:6]...)
	add := func(dir int, seq byte, b []byte) { steps = append(steps, sessionStep{dir, mysqlTestPacket(seq, b)}) }
	col := []byte{3, 'd', 'e', 'f', 0, 0, 0, 1, 'x', 0, 12, 33, 0, 4, 0, 0, 0, 3, 0, 0, 0, 0, 0}
	eof := func(status byte) []byte {
		b := []byte{0xfe, 0, 0, status, 0}
		if deprecated {
			b = append(b, 0, 0)
		}
		return b
	}
	add(0, 0, []byte{0x16, 'S', 'E', 'L', 'E', 'C', 'T', ' ', '?'})
	add(1, 1, []byte{0, 1, 0, 0, 0, 1, 0, 1, 0, 0, 0, 0})
	add(1, 2, col)
	seq := byte(3)
	if !deprecated {
		add(1, seq, eof(2))
		seq++
	}
	add(1, seq, col)
	seq++
	if !deprecated {
		add(1, seq, eof(2))
	}
	for round := 0; round < 2; round++ {
		flags := byte(0)
		if cursor {
			flags = 1
		}
		body := []byte{0x17, 1, 0, 0, 0, flags, 1, 0, 0, 0, 0, 0}
		if round == 0 {
			body[11] = 1
			body = append(body, 3, 0)
		}
		body = binary.LittleEndian.AppendUint32(body, uint32(42+round))
		add(0, 0, body)
		add(1, 1, []byte{1})
		add(1, 2, col)
		seq = 3
		if !deprecated {
			status := byte(2)
			if cursor {
				status |= 0x40
			}
			add(1, seq, eof(status))
			seq++
		}
		if cursor {
			add(0, 0, []byte{0x1c, 1, 0, 0, 0, 1, 0, 0, 0})
			seq = 1
		}
		add(1, seq, []byte{0, 0, byte(42 + round), 0, 0, 0})
		seq++
		status := byte(2)
		if cursor {
			status |= 0x80
		}
		add(1, seq, eof(status))
		if round == 0 {
			add(0, 0, []byte{0x1a, 1, 0, 0, 0})
			add(1, 1, []byte{0, 0, 0, 2, 0, 0, 0})
		}
	}
	add(0, 0, []byte{0x19, 1, 0, 0, 0})
	return steps
}
func TestM2MySQLPreparedState(t *testing.T) {
	for _, mode := range []struct{ deprecated, cursor bool }{{false, false}, {true, false}, {false, true}} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			steps := m2bPreparedSteps(mode.deprecated, mode.cursor)
			for _, chunk := range []int{0, 1, 7} {
				ev, _ := sessionTestFlow(t, "mysql", steps, chunk, true)
				assertSessionEvents(t, ev, "mysql", true)
				require.Len(t, m2bRows(ev), 2)
				require.Contains(t, fmt.Sprint(m2bRows(ev)), `"Value":43`)
			}
			// Every cut of every PDU uses the public flow path, preserving all other PDUs.
			for i, s := range steps {
				for cut := 1; cut < len(s.wire); cut++ {
					split := append([]sessionStep(nil), steps[:i]...)
					split = append(split, sessionStep{s.dir, s.wire[:cut]}, sessionStep{s.dir, s.wire[cut:]})
					split = append(split, steps[i+1:]...)
					ev, _ := sessionTestFlow(t, "mysql", split, 0, false)
					for _, e := range ev {
						require.Equal(t, "decoded", e.Status, "PDU %d cut %d: %s", i, cut, e.Error)
					}
					require.Len(t, m2bRows(ev), 2)
				}
			}
			steps = append(steps, sessionStep{0, mysqlTestPacket(0, []byte{0x1a, 1, 0, 0, 0})})
			ev, _ := sessionTestFlow(t, "mysql", steps, 0, false)
			require.Equal(t, "context-required", ev[len(ev)-1].Status)
		})
	}
	t.Run("error-mid-result", func(t *testing.T) {
		steps := m2bPreparedSteps(false, false)
		for i, s := range steps {
			if s.dir == 1 && s.wire[3] == 4 && len(s.wire) == 10 && s.wire[4] == 0 {
				steps = steps[:i]
				steps = append(steps, sessionStep{1, mysqlTestPacket(4, []byte{0xff, 0x28, 4, '#', 'H', 'Y', '0', '0', '0', 'e'})}, sessionStep{0, mysqlTestPacket(0, []byte{0x19, 1, 0, 0, 0})})
				break
			}
		}
		ev, _ := sessionTestFlow(t, "mysql", steps, 1, false)
		assertSessionEvents(t, ev, "mysql", false)
		require.Equal(t, uint16(1064), ev[len(ev)-2].Session["Error Code"])
		require.Equal(t, "COM_STMT_CLOSE", ev[len(ev)-1].Session["Command"])
	})
	t.Run("binary-scalars", func(t *testing.T) {
		for _, tc := range []struct {
			typ   byte
			wire  []byte
			u     bool
			value any
		}{{1, []byte{255}, false, int64(-1)}, {8, []byte{255, 255, 255, 255, 255, 255, 255, 255}, true, ^uint64(0)}, {253, []byte{0}, false, []byte{}}, {10, []byte{0}, false, map[string]any{"Zero": true}}} {
			v, n, e := stream_parser.DecodeMySQLBinaryValue(tc.wire, tc.typ, tc.u)
			require.NoError(t, e)
			require.Equal(t, len(tc.wire), n)
			require.Equal(t, tc.value, v)
			for cut := 0; cut < len(tc.wire); cut++ {
				_, _, err := stream_parser.DecodeMySQLBinaryValue(tc.wire[:cut], tc.typ, tc.u)
				require.Error(t, err)
			}
		}
	})
}
func TestM2PostgresPipelinedPortal(t *testing.T) {
	startup := pgSessionUntyped(196608, []byte("user\x00m2\x00\x00"))
	parse := pgSessionMsg('P', []byte("s\x00SELECT 42\x00\x00\x00"))
	bind := pgSessionMsg('B', []byte("p\x00s\x00\x00\x00\x00\x00\x00\x01\x00\x01"))
	desc := pgSessionMsg('D', []byte("Pp\x00"))
	exec := pgSessionMsg('E', []byte("p\x00\x00\x00\x00\x01"))
	sync := pgSessionMsg('S', nil)
	col := []byte{0, 1, 'n', 0}
	col = append(col, make([]byte, 6)...)
	col = binary.BigEndian.AppendUint32(col, 23)
	col = binary.BigEndian.AppendUint16(col, 4)
	col = binary.BigEndian.AppendUint32(col, ^uint32(0))
	col = binary.BigEndian.AppendUint16(col, 1)
	row := pgSessionMsg('D', []byte{0, 1, 0, 0, 0, 4, 0, 0, 0, 42})
	steps := []sessionStep{{0, startup}, {1, pgSessionMsg('R', []byte{0, 0, 0, 0})}, {1, pgSessionMsg('Z', []byte{'I'})}, {0, parse}, {0, bind}, {0, desc}, {0, exec}, {0, pgSessionMsg('H', nil)}, {1, pgSessionMsg('1', nil)}, {1, pgSessionMsg('2', nil)}, {1, pgSessionMsg('T', col)}, {1, row}, {1, pgSessionMsg('s', nil)}, {0, exec}, {0, sync}, {1, row}, {1, pgSessionMsg('C', []byte("SELECT 2\x00"))}, {1, pgSessionMsg('Z', []byte{'I'})}}
	steps = append(steps, sessionStep{0, pgSessionMsg('C', []byte("Ss\x00"))}, sessionStep{0, sync}, sessionStep{1, pgSessionMsg('3', nil)}, sessionStep{1, pgSessionMsg('Z', []byte{'I'})})
	for _, chunk := range []int{0, 1, 7} {
		ev, _ := sessionTestFlow(t, "postgresql", steps, chunk, true)
		assertSessionEvents(t, ev, "postgresql", true)
		rows := 0
		for _, e := range ev {
			if e.Session["Message Name"] == "DataRow" {
				rows++
				require.Equal(t, "s", e.Session["Statement"])
				require.Equal(t, "p", e.Session["Portal"])
				vals := e.Session["Values"].([]map[string]any)
				require.Equal(t, int64(42), vals[0]["Decoded int4"])
				require.NotZero(t, e.ResponseTo)
			}
		}
		require.Equal(t, 2, rows)
	}
}
func TestM2TLSApplicationHandoff(t *testing.T) {
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	f := &binFlow{a: c.binParser, id: 1, protocol: "tls", mysql: &binMySQL{phase: "tls", server: 1, seq: 2, serverCaps: uint64(mysqlTestCaps)}}
	_, response := mysqlTestHandshake(mysqlTestCaps)
	response[3] = 2
	f.tls = &binTLS{plaintext: response}
	f.deliverTLS(0, &ProtocolEvent{ID: 9, Timestamp: time.Unix(1, 0)})
	require.Equal(t, "mysql", f.tls.child.protocol)
	require.Equal(t, "auth-server", f.tls.child.mysql.phase)
	require.Nil(t, f.mysql)
	f.close(TrafficFlowCloseReason_FIN)
	require.Zero(t, c.binParser.stats().BufferedBytes)
}

func FuzzFirstBatchM2B(f *testing.F) {
	for _, w := range [][]byte{{0}, {1, 0, 0, 0}, {0, 1, 0, 0, 0, 4, 0, 0, 0, 42}, h3RequestHeaders(), rfcHex("03811011")} {
		f.Add(w)
	}
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 4096 {
			return
		}
		for _, typ := range []byte{1, 2, 3, 4, 5, 8, 10, 11, 12, 253} {
			_, _, _ = stream_parser.DecodeMySQLBinaryValue(w, typ, false)
		}
		_, _ = quicParseFrames(w, 64)
		var a quicAssembler
		if len(w) > 1 {
			_, _, _, _ = a.feed(uint64(w[0]), w[1:], true, 4096, 64)
			_, _, _, _ = a.feed(0, w[1:], false, 4096, 64)
		}
		q := &binQUIC{native: true, byteLimit: 4096, alpn: "h3"}
		_ = q.feedHTTP3(0, 0, 0, w, true, 64, map[string]any{}, map[string]any{})
		q = &binQUIC{native: true, byteLimit: 4096, alpn: "doq"}
		_ = q.feedDoQ(0, 0, 0, w, true, 64, map[string]any{}, map[string]any{})
	})
}

func TestM2QUICApplicationBoundaries(t *testing.T) {
	t.Run("reset-final-size", func(t *testing.T) {
		q := &binQUIC{native: true, byteLimit: 128}
		_, e := q.finishFrames(0, quicSpaceApplication, []map[string]any{{"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": uint64(0), "Stream Data": []byte("abc"), "FIN": false}}, map[string]any{}, 4)
		require.NoError(t, e)
		_, e = q.finishFrames(0, quicSpaceApplication, []map[string]any{{"Frame Type": "RESET_STREAM", "Stream ID": uint64(0), "Final Size": uint64(2)}}, map[string]any{}, 4)
		require.Error(t, e)
	})
	t.Run("cid-budget-and-retirement", func(t *testing.T) {
		q := &binQUIC{native: true, cids: map[string]int{}}
		frame := func(seq, retire uint64, cid string) map[string]any {
			return map[string]any{"Frame Type": "NEW_CONNECTION_ID", "Sequence": seq, "Retire Prior To": retire, "CID": []byte(cid)}
		}
		require.NoError(t, q.applyFrames(0, 2, []map[string]any{frame(1, 0, "a"), frame(2, 0, "b")}, map[string]any{}, 2))
		require.Error(t, q.applyFrames(0, 2, []map[string]any{frame(3, 0, "c")}, map[string]any{}, 2))
		require.NoError(t, q.applyFrames(0, 2, []map[string]any{frame(3, 2, "c")}, map[string]any{}, 2))
		require.NotContains(t, q.cids, "a")
		require.Error(t, q.applyFrames(0, 2, []map[string]any{frame(3, 2, "x")}, map[string]any{}, 2))
	})
	t.Run("http3-duplicate-control", func(t *testing.T) {
		q := &binQUIC{native: true, byteLimit: 4096}
		require.NoError(t, q.feedHTTP3(0, 2, 0, h3ControlSETTINGS(1, 220), false, 64, map[string]any{}, map[string]any{}))
		require.Error(t, q.feedHTTP3(0, 6, 0, h3ControlSETTINGS(1, 220), false, 64, map[string]any{}, map[string]any{}))
	})
	t.Run("doq-fin-and-message-boundaries", func(t *testing.T) {
		msg := []byte{0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 'x', 0, 0, 1, 0, 1}
		wire := binary.BigEndian.AppendUint16(nil, uint16(len(msg)))
		wire = append(wire, msg...)
		for _, bad := range [][]byte{wire[:len(wire)-1], append(append([]byte(nil), wire...), wire...), append([]byte{0, 19, 0, 1}, wire[4:]...)} {
			q := &binQUIC{native: true, byteLimit: 4096}
			require.Error(t, q.feedDoQ(0, 0, 0, bad, true, 64, map[string]any{}, map[string]any{}))
		}
		q := &binQUIC{native: true, byteLimit: 4096}
		info := map[string]any{}
		require.NoError(t, q.feedDoQ(0, 0, 0, wire, false, 64, map[string]any{}, info))
		require.NotEqual(t, true, info["DoQ"])
		info = map[string]any{}
		require.NoError(t, q.feedDoQ(0, 0, uint64(len(wire)), nil, true, 64, map[string]any{}, info))
		require.Equal(t, true, info["DoQ"])
	})
}

func TestM2PostgresCopyBothAndAsync(t *testing.T) {
	steps := []sessionStep{{0, pgSessionUntyped(196608, []byte("user\x00m2\x00\x00"))}, {1, pgSessionMsg('R', []byte{0, 0, 0, 0})}, {1, pgSessionMsg('Z', []byte{'I'})}, {0, pgSessionMsg('Q', []byte("START_REPLICATION\x00"))}, {1, pgSessionMsg('W', []byte{0, 0, 0})}, {1, pgSessionMsg('A', []byte{0, 0, 0, 1, 'c', 0, 'v', 0})}, {0, pgSessionMsg('d', []byte("client"))}, {1, pgSessionMsg('d', []byte("server"))}, {0, pgSessionMsg('c', nil)}, {1, pgSessionMsg('c', nil)}, {1, pgSessionMsg('C', []byte("COPY 1\x00"))}, {1, pgSessionMsg('Z', []byte{'I'})}}
	for _, chunk := range []int{0, 1, 7} {
		ev, _ := sessionTestFlow(t, "postgresql", steps, chunk, true)
		assertSessionEvents(t, ev, "postgresql", true)
		require.Equal(t, true, ev[5].Session["Asynchronous"])
		require.Zero(t, ev[5].ResponseTo)
		for _, i := range []int{6, 7, 8, 9} {
			require.Equal(t, "both", ev[i].Session["Copy Mode"])
			require.Equal(t, ev[3].ID, ev[i].ResponseTo)
		}
		require.Equal(t, [2]uint64{6, 6}, ev[9].Session["Copy Bytes"])
		require.Equal(t, [2]bool{true, true}, ev[9].Session["Copy Done"])
		require.Equal(t, 0, ev[11].Session["Outstanding"])
	}
	bad := append([]sessionStep(nil), steps[:5]...)
	bad[4].wire = pgSessionMsg('G', []byte{0, 0, 0})
	bad = append(bad, sessionStep{1, pgSessionMsg('d', []byte("wrong direction"))})
	ev, _ := sessionTestFlow(t, "postgresql", bad, 1, false)
	require.Equal(t, "context-required", ev[len(ev)-1].Status)
}

func TestM2QUICRepeatedKeyUpdate(t *testing.T) {
	ring, err := quicLoadKeys(QUICKeyMaterial{Secrets: map[string][]byte{"CLIENT_TRAFFIC_SECRET_0": bytes.Repeat([]byte{7}, 32)}})
	require.NoError(t, err)
	keys := []*quicTrafficKeys{ring.app[0][0], ring.app[0][1]}
	for len(keys) < 4 {
		k, e := quicNextPhase(keys[len(keys)-1])
		require.NoError(t, e)
		keys = append(keys, k)
	}
	packet := func(generation int, pn uint64) []byte {
		k := keys[generation]
		header := []byte{0x43 | byte(generation%2)<<2, 'c', 'i', 'd'}
		header = binary.BigEndian.AppendUint32(header, uint32(pn))
		block, e := aes.NewCipher(k.key)
		require.NoError(t, e)
		a, e := cipher.NewGCM(block)
		require.NoError(t, e)
		raw := append(append([]byte(nil), header...), a.Seal(nil, quicNonce(k.iv, pn), []byte{1, 0, 0, 0}, header)...)
		mask, e := quicHeaderMask(k, raw[8:24])
		require.NoError(t, e)
		raw[0] ^= mask[0] & 0x1f
		for i := 0; i < 4; i++ {
			raw[4+i] ^= mask[i+1]
		}
		return raw
	}
	q := &binQUIC{native: true, byteLimit: 4096, keys: ring, cids: map[string]int{"cid": 1}}
	for _, pair := range [][2]int{{0, 1}, {1, 3}, {2, 4}, {1, 2}, {3, 5}} {
		info, e := q.consume(0, packet(pair[0], uint64(pair[1])), 64)
		require.NoError(t, e)
		require.Equal(t, pair[0]%2, info["Key Phase"])
		require.Equal(t, true, info["Authentication Verified"])
	}
	require.Equal(t, 1, q.keyPhase[0])
	bad := packet(3, 6)
	bad[len(bad)-1] ^= 1
	_, err = q.consume(0, bad, 64)
	require.Error(t, err)
	require.Equal(t, uint64(5), q.spaces[0][2].largest)
}

func TestM2MySQLPreparedMultipleResults(t *testing.T) {
	for _, capable := range []bool{false, true} {
		steps := m2bPreparedSteps(false, false)
		if capable {
			g, r := mysqlTestHandshake(mysqlTestCaps | (1 << 18))
			steps[0].wire = g
			steps[1].wire = r
		}
		executed := false
		for i, s := range steps {
			if s.dir == 0 && s.wire[4] == 0x17 {
				executed = true
			}
			if executed && s.dir == 1 && s.wire[3] == 5 && len(s.wire) == 9 {
				s.wire = append([]byte(nil), s.wire...)
				s.wire[7] |= 8
				steps[i] = s
				steps = append(append(append([]sessionStep(nil), steps[:i+1]...), sessionStep{1, mysqlTestPacket(6, []byte{0, 0, 0, 2, 0, 0, 0})}), steps[i+1:]...)
				break
			}
		}
		ev, _ := sessionTestFlow(t, "mysql", steps, 1, false)
		if capable {
			assertSessionEvents(t, ev, "mysql", false)
			require.Len(t, m2bRows(ev), 2)
		} else {
			found := false
			for _, e := range ev {
				if e.Status == "malformed" {
					found = true
					require.Contains(t, e.Error, "multiple results")
				}
			}
			require.True(t, found)
		}
	}
}

func TestM2HTTP3InformationalResponse(t *testing.T) {
	// QPACK literal name and value: :status = 103 (no dynamic table).
	early := h3Frame(1, append([]byte{0, 0, 0x27, 0}, append([]byte(":status"), []byte{3, '1', '0', '3'}...)...))
	for _, withFinal := range []bool{false, true} {
		q := &binQUIC{native: true, byteLimit: 4096}
		require.NoError(t, q.feedHTTP3(0, 0, 0, h3RequestHeaders(), true, 64, map[string]any{}, map[string]any{}))
		require.NoError(t, q.feedHTTP3(1, 0, 0, early, false, 64, map[string]any{}, map[string]any{}))
		require.Zero(t, q.streams[0].h3.headers[1])
		body := h3Data([]byte("ok"))
		if withFinal {
			body = append(h3ResponseHeaders(), body...)
		}
		err := q.feedHTTP3(1, 0, uint64(len(early)), body, true, 64, map[string]any{}, map[string]any{})
		if withFinal {
			require.NoError(t, err)
			require.Equal(t, 1, q.streams[0].h3.respHeads)
		} else {
			require.Error(t, err)
		}
	}
}
