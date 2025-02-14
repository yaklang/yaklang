package har

import (
	jlexer "github.com/mailru/easyjson/jlexer"
	jwriter "github.com/mailru/easyjson/jwriter"
	"github.com/samber/lo"
)

func easyjson46e2e00bDecodeGithubComYaklangYaklangCommonHar13(in *jlexer.Lexer, out *Entries) {
	in.Delim('[')
	if !in.IsDelim(']') {
		out.Entries = make([]*HAREntry, 0, 8)
	} else {
		out.Entries = make([]*HAREntry, 0)
	}
	for !in.IsDelim(']') {
		var v2 *HAREntry
		if in.IsNull() {
			in.Skip()
			v2 = nil
		} else {
			v2 = new(HAREntry)
			(*v2).UnmarshalEasyJSON(in)
			if out.unmarshalEntryCallback != nil {
				err := out.unmarshalEntryCallback(v2)
				if err != nil {
					in.AddError(err)
				}
			}
		}
		out.Entries = append(out.Entries, v2)
		in.WantComma()
	}
	in.Delim(']')
}

func easyjson46e2e00bEncodeGithubComYaklangYaklangCommonHar13(out *jwriter.Writer, in Entries) {
	first := true
	_ = first
	if in.Entries == nil && in.entriesChannel == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
		out.RawString("null")
	} else {
		out.RawByte('[')
		if in.entriesChannel == nil {
			ch := lo.SliceToChannel(0, in.Entries)
			in.entriesChannel = ch
		}

		for entry := range in.entriesChannel {
			if first {
				first = false
			} else {
				out.RawByte(',')
			}
			if entry == nil {
				out.RawString("null")
			} else {
				(*entry).MarshalEasyJSON(out)
			}
			if in.marshalEntryCallback != nil {
				in.marshalEntryCallback(entry)
			}
		}
		out.RawByte(']')
	}
}
