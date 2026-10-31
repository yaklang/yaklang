package pcaputil

import "github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"

// Called only after request identity, capture domain, generation and direction
// were matched. The walker has already converted envelope ranges to the whole
// owned Direct TCP PDU; adding header or compound offsets again would be wrong.
func smb2DecodeMatchedQueryOutput(command map[string]any, selector smb2QueryInfoSelector) {
	profile, supported := stream_parser.SMB2QueryInfoOutputProfile(selector.infoType, selector.fileInfoClass)
	if !supported {
		return
	}
	// The class name is request context until a complete output is decoded.
	// Do not give the selector or its name a fabricated response wire range.
	command["Request Output Information Type"] = profile
	status, _ := command["Status"].(uint32)
	partial := status == 0x80000005
	command["Output Content Partial"] = partial
	body, ok := command["Body"].(map[string]any)
	if !ok {
		return
	}
	output, _ := body["Output Buffer"].([]byte)
	decoded, err := stream_parser.DecodeSMB2QueryInfoOutput(output, selector.infoType, selector.fileInfoClass)
	if err != nil {
		// Proven SMB2 and its complete envelope remain visible. A bad selected
		// payload must not discard other commands or poison connection identity.
		command["Output Content Status"] = "opaque: selected output is incomplete or malformed"
		if partial {
			command["Output Content Status"] = "opaque partial output: selected record is incomplete or malformed"
		}
		command["Output Decode Error"] = err.Error()
		return
	}
	ranges, _ := body["Wire Field Ranges"].(map[string]any)
	span, _ := ranges["Output Buffer"].([]any)
	if len(span) != 2 {
		command["Output Content Status"] = "opaque: output source range unavailable"
		return
	}
	lo, loOK := displayUint(span[0])
	hi, hiOK := displayUint(span[1])
	if !loOK || !hiOK || hi < lo || hi-lo != uint64(len(output)) || uint64(int(lo)) != lo || int(lo) < 0 {
		command["Output Content Status"] = "opaque: output source range inconsistent"
		return
	}
	smb2OffsetRanges(decoded, int(lo))
	body[profile] = decoded
	ranges[profile] = []any{lo, hi}
	command["Output Information Type"] = profile
	command["Output Content Status"] = "decoded: selected output from matched request"
	if partial {
		command["Output Content Status"] = "decoded partial output: server reports buffer overflow"
	}
}
