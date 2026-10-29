// External direct-codec probe. No native-ingress or packet-label equivalence
// is implied by a caller-supplied budget or unavailable-pseudo-header flag.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type control struct {
	Name string `json:"name"`
	Frame string `json:"original_frame"`
	FrameSHA string `json:"original_frame_SHA256"`
	Direct bool `json:"requires_direct_codec_invocation"`
	Budget int `json:"raw_codec_collection_budget"`
	Context struct { Unavailable bool `json:"PseudoHeaderUnavailable"` } `json:"raw_codec_context_override"`
}

func errorTree(err error) any {
	if err == nil { return nil }
	children := []any{}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		for _, child := range wrapped.Unwrap() { children = append(children,errorTree(child)) }
	case interface{ Unwrap() error }:
		children = append(children,errorTree(wrapped.Unwrap()))
	}
	return map[string]any{"type":fmt.Sprintf("%T",err),"text":err.Error(),"children":children}
}

func main() {
	if len(os.Args)!=2 { panic("usage: direct-codec-probe cases.json") }
	data,err:=os.ReadFile(os.Args[1]);if err!=nil { panic(err) }
	var cases []control;if err=json.Unmarshal(data,&cases);err!=nil { panic(err) }
	encoder:=json.NewEncoder(os.Stdout)
	for _,c:=range cases {
		if !c.Direct { continue }
		raw,err:=os.ReadFile(c.Frame);if err!=nil { panic(err) }
		sum:=sha256.Sum256(raw);if hex.EncodeToString(sum[:])!=c.FrameSHA { panic("source hash mismatch") }
		// Frozen authored controls use Ethernet/IPv6/NH89 without extensions.
		if len(raw)<54 || raw[12]!=0x86 || raw[13]!=0xdd || raw[14]>>4!=6 || raw[20]!=89 { panic("unsupported direct-control carrier") }
		source,ok:=netip.AddrFromSlice(raw[22:38]);if !ok { panic("bad IPv6 source") }
		destination,ok:=netip.AddrFromSlice(raw[38:54]);if !ok { panic("bad IPv6 destination") }
		budget:=c.Budget;if budget==0 { budget=4096 }
		context:=stream_parser.OSPFv3WireContext{Source:source,Destination:destination,PseudoHeaderUnavailable:c.Context.Unavailable}
		fields,decodeErr:=stream_parser.DecodeOSPFv3Wire(raw[54:],context,budget)
		text:="";if decodeErr!=nil { text=decodeErr.Error() }
		err=encoder.Encode(map[string]any{"case":c.Name,"source_frame_SHA256":c.FrameSHA,"budget":budget,"pseudo_header_unavailable":c.Context.Unavailable,"fields":fields,"error":text,"budget_identity":errors.Is(decodeErr,stream_parser.ErrObservedWireBudget),"context_identity":errors.Is(decodeErr,stream_parser.ErrObservedWireContext),"integrity_identity":errors.Is(decodeErr,stream_parser.ErrOSPFv3WireIntegrity),"error_tree":errorTree(decodeErr),"direct_codec_only":true})
		if err!=nil { panic(err) }
	}
}
