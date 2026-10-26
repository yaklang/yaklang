package pcaputil

import (
	"bytes"
	"testing"
)

// Exercise post-authentication STREAM delivery under arbitrary loss, ordering,
// overlap and multiplexing. A separate byte-presence model determines the
// contiguous prefix; each reported witness must contain those literal bytes.
// This does not substitute for wire AEAD or reference-capture verification.
func FuzzQUICStreamSourceCoverage(f *testing.F) {
	f.Add([]byte{0, 4, 3, 0, 0, 0, 3, 0})
	f.Add([]byte{16, 63, 31, 0, 1, 0, 31, 0, 2, 17, 22, 0})
	f.Fuzz(func(t *testing.T, ops []byte) {
		if len(ops) > 512 {
			ops = ops[:512]
		}
		q := &binQUIC{native: true, byteLimit: 1 << 20}
		type sentRange struct {
			sid, off uint64
			dir      int
			data     []byte
		}
		sent := map[uint64]sentRange{}
		var seen [3][2][96]bool
		var prefix [3][2]int
		pdu := uint64(0)
		feed := func(stream, dir, off, length int) {
			pdu++
			sid := uint64(stream * 4)
			data := make([]byte, length)
			for i := range data {
				data[i] = byte(stream*3 + dir*11 + (off+i)*7)
				seen[stream][dir][off+i] = true
			}
			old := prefix[stream][dir]
			for prefix[stream][dir] < 96 && seen[stream][dir][prefix[stream][dir]] {
				prefix[stream][dir]++
			}
			sent[pdu] = sentRange{sid, uint64(off), dir, bytes.Clone(data)}
			q.eventID, q.eventRefs = pdu, []PacketReference{{Number: pdu, Domain: CaptureDomain{Interface: 17}}}
			fr := map[string]any{"Frame Type": "STREAM", "Stream ID": sid, "Offset": uint64(off), "Stream Data": data, "FIN": false}
			if err := q.applyFrames(dir, quicSpaceApplication, []map[string]any{fr}, map[string]any{}, 1024); err != nil {
				t.Fatal(err)
			}
			if got, want := fr["Delivered Bytes"].(int), prefix[stream][dir]-old; got != want {
				t.Fatalf("contiguous delivery: got %d want %d", got, want)
			}
			source := fr["Byte Source"].(map[string]any)
			if source["Offset"] != uint64(off) || source["Length"] != uint64(length) {
				t.Fatal("raw STREAM source differs from literal frame")
			}
			if prefix[stream][dir] == old {
				return
			}
			source = fr["Delivered Byte Source"].(map[string]any)
			if source["Offset"] != uint64(old) || source["Length"] != uint64(prefix[stream][dir]-old) || source["Coverage"] != "complete" {
				t.Fatal("complete delivery must retain its exact source range")
			}
			var witnessed [96]bool
			for _, r := range source["Ranges"].([]map[string]any) {
				parent := r["Parent PDU"].(uint64)
				s, ok := sent[parent]
				x, n := r["Offset"].(uint64), r["Length"].(uint64)
				if !ok || parent > pdu || s.sid != sid || s.dir != dir || x < s.off || x+n > s.off+uint64(len(s.data)) || x < uint64(old) || x+n > uint64(prefix[stream][dir]) {
					t.Fatal("witness points outside observed source or delivery")
				}
				refs := r["Packet References"].([]map[string]any)
				if len(refs) != 1 || refs[0]["Packet"] != parent || refs[0]["Interface"] != 17 {
					t.Fatal("witness physical reference differs from original observation")
				}
				for i := x; i < x+n; i++ {
					if s.data[i-s.off] != byte(stream*3+dir*11+int(i)*7) {
						t.Fatal("witness bytes differ")
					}
					witnessed[i] = true
				}
			}
			for i := old; i < prefix[stream][dir]; i++ {
				if !witnessed[i] {
					t.Fatal("unwitnessed delivered byte")
				}
			}
		}
		for i := 0; i+3 < len(ops); i += 4 {
			feed(int(ops[i])%3, int(ops[i]>>4)&1, int(ops[i+1])%64, 1+int(ops[i+2])%32)
		}
		// A later retransmission fills every remaining hole independently for
		// each stream/direction; no sticky proof may be borrowed from a peer.
		for stream := 0; stream < 3; stream++ {
			for dir := 0; dir < 2; dir++ {
				feed(stream, dir, 0, 96)
			}
		}
	})
}
