package pcaputil

import (
	"reflect"
	"testing"
)

func FuzzPacketQueryRevisionPreservesSnapshot(f *testing.F) {
	f.Add([]byte{1, 7, 4, 0, 2, 7, 2, 1, 3, 9, 4, 0})
	f.Add([]byte{0, 1, 1, 1, 2, 2, 4, 0})
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 512 {
			input = input[:512]
		}
		v, err := NewProtocolInspector(8, 4096)
		if err != nil {
			t.Fatal(err)
		}
		if len(input) > 0 && input[0]&1 != 0 {
			v.packetIndexLimit = 4
		}
		ref := PacketReference{Number: 1, Domain: CaptureDomain{Interface: 1}}
		initial := []uint64{1, 2}
		physical := len(input) > 0 && input[0]&2 != 0
		type value struct {
			ID      uint64
			Present bool
			Type    any
			Raw     string
		}
		snapshot := func() any {
			ids, complete := v.PacketEventWindow(ref, physical)
			ids = append(ids, initial...)
			values := make([]value, 0, len(ids))
			for _, id := range ids {
				e, _ := v.Details(id)
				if e == nil {
					values = append(values, value{ID: id})
					continue
				}
				values = append(values, value{ID: id, Present: true, Type: e.Session["Type"], Raw: string(e.Raw)})
			}
			return struct {
				Complete bool
				Values   []value
			}{complete, values}
		}
		for at := 0; at+3 < len(input); at += 4 {
			before := v.PacketQueryRevision(ref, initial, physical)
			old := snapshot()
			owner := PacketReference{Number: uint64(input[at]&3) + 1, Domain: CaptureDomain{Interface: int(input[at+1] & 3)}}
			if input[at]&4 != 0 {
				owner.Domain.Encapsulation = "/gre:observed"
			}
			e := &ProtocolEvent{ID: uint64(input[at+2] & 15), Protocol: "bgp", Session: map[string]any{"Type": uint64(input[at+3] & 7)}, Raw: []byte{input[at+3]}}
			if input[at]&8 != 0 {
				e.SourceBytes.PacketRefs = []PacketReference{owner, owner}
			}
			v.OnEvent(e)
			if before == v.PacketQueryRevision(ref, initial, physical) && !reflect.DeepEqual(old, snapshot()) {
				t.Fatal("unchanged query revision hid a changed retained snapshot")
			}
			if v.packetIndexLinks > v.packetIndexLimit || v.unindexed < 0 {
				t.Fatal("reference indexing exceeded its bounded accounting")
			}
		}
	})
}
