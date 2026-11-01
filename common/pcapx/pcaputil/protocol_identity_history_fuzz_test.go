package pcaputil

import (
	"reflect"
	"testing"
)

func FuzzContributorIdentitySourceAndCoverage(f *testing.F) {
	f.Add([]byte{1, 7, 4, 0, 1, 7, 2, 1, 3, 9, 4, 0})
	f.Add([]byte{0, 1, 1, 1, 2, 2, 4, 0, 255, 255, 255, 255})
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 512 {
			input = input[:512]
		}
		v, err := NewProtocolInspector(4, 1024)
		if err != nil {
			t.Fatal(err)
		}
		v.identities = newProtocolIdentityIndex(2048)
		query := PacketReference{Number: 1, Domain: CaptureDomain{Interface: 1}}
		physical := len(input) > 0 && input[0]&1 != 0
		type fact struct {
			id, arrival uint64
			ref         PacketReference
		}
		var source []fact
		for i := 0; i+3 < len(input); i += 4 {
			before := v.PacketQueryRevision(query, nil, physical)
			old := v.PacketContributorWindow(query, physical, 4096)
			ref := PacketReference{Number: uint64(input[i]&7) + 1, Domain: CaptureDomain{Interface: int(input[i+1] & 3)}}
			if input[i]&8 != 0 {
				ref.Domain.Encapsulation = "/gre:observed"
			}
			id := uint64(input[i+2]&15) + 1
			e := identityObservation(id, ref, ref)
			if input[i+3]&1 != 0 {
				e.Raw = make([]byte, 2048)
			}
			v.OnEvent(e)
			source = append(source, fact{id, v.Revision(), ref})
			e.ID, e.SourceBytes.PacketRefs[0].Number = 999, 999
			window := v.PacketContributorWindow(query, physical, 4096)
			belongs := func(ref PacketReference) bool {
				if physical {
					return physicalReference(ref) == physicalReference(query)
				}
				return ref == query
			}
			for _, proof := range window.Identities {
				found := false
				for _, original := range source {
					if original.id == proof.ID && original.arrival == proof.Arrival && belongs(original.ref) {
						found = true
						break
					}
				}
				if !found {
					t.Fatal("identity not witnessed by the original callback/source domain")
				}
			}
			if window.Complete {
				for _, original := range source {
					if !belongs(original.ref) {
						continue
					}
					found := false
					for _, proof := range window.Identities {
						if proof.ID == original.id && proof.Arrival == original.arrival {
							found = true
							break
						}
					}
					if !found {
						t.Fatal("complete identity window omitted an actual contributor")
					}
				}
			}
			_, complete := v.PacketEventWindow(query, physical)
			if complete {
				for _, original := range source {
					if !belongs(original.ref) {
						continue
					}
					detail, _ := v.Details(original.id)
					if detail == nil || detail.HistoryArrival() != original.arrival {
						t.Fatal("complete field window borrowed absent/duplicate details")
					}
				}
			}
			if before == v.PacketQueryRevision(query, nil, physical) && !reflect.DeepEqual(old, window) {
				t.Fatal("unchanged revision hid changed identity evidence")
			}
			x := v.identities
			if x.bytes > x.maxBytes || x.count > len(x.rows) {
				t.Fatal("identity storage exceeded its budget")
			}
		}
	})
}
