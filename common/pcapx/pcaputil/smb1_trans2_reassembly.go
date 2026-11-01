package pcaputil

import (
	"bytes"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/log"
)

// Logical TRANSACTION2 bytes. Each slice keeps the displacement from its own
// SMB. The concatenated buffer is observed context and has no single PDU range.
type smb1Trans2Buffer struct {
	totalP, totalD int
	params, data   []smb1Trans2Span
	unicode        bool
	publications   int
	paramsOut      []byte
	dataOut        []byte
}

type smb1Trans2Span struct {
	at int
	b  []byte
}

func smb1Trans2Start(header, body map[string]any) *smb1Trans2Buffer {
	flags2, _ := header["Flags 2"].(uint16)
	buf := &smb1Trans2Buffer{
		totalP:  int(smb1MapUint16(body["Total Parameter Count"])),
		totalD:  int(smb1MapUint16(body["Total Data Count"])),
		unicode: flags2&0x8000 != 0,
	}
	if raw, ok := body["Parameters"].([]byte); ok {
		_, _ = buf.add(true, 0, raw)
	}
	if raw, ok := body["Data"].([]byte); ok {
		_, _ = buf.add(false, 0, raw)
	}
	if buf.covered(true) && buf.covered(false) {
		buf.publications = 1
		buf.paramsOut = smb1Materialize(buf.params, buf.totalP)
		buf.dataOut = smb1Materialize(buf.data, buf.totalD)
	}
	return buf
}

func (b *smb1Trans2Buffer) clone() *smb1Trans2Buffer {
	if b == nil {
		return nil
	}
	out := *b
	out.params = smb1CloneSpans(b.params)
	out.data = smb1CloneSpans(b.data)
	out.paramsOut = bytes.Clone(b.paramsOut)
	out.dataOut = bytes.Clone(b.dataOut)
	return &out
}

func (b *smb1Trans2Buffer) retained() int64 {
	if b == nil {
		return 0
	}
	n := int64(len(b.paramsOut) + len(b.dataOut))
	for _, span := range append(append([]smb1Trans2Span{}, b.params...), b.data...) {
		n += int64(len(span.b) + 16)
	}
	return n
}

func (b *smb1Trans2Buffer) admit(body map[string]any) (string, error) {
	param, _ := body["Parameters"].([]byte)
	data, _ := body["Data"].([]byte)
	pStatus, err := b.add(true, int(smb1MapUint16(body["Parameter Displacement"])), param)
	if err != nil {
		return "", err
	}
	dStatus, err := b.add(false, int(smb1MapUint16(body["Data Displacement"])), data)
	if err != nil {
		return "", err
	}
	switch {
	case pStatus == "overlap conflict" || dStatus == "overlap conflict":
		log.Debugf("smb1: TRANSACTION2 secondary overlapped a different byte")
		return "overlap conflict", nil
	case pStatus == "hole" || dStatus == "hole":
		log.Debugf("smb1: TRANSACTION2 secondary left a hole in the logical buffer")
		return "hole", nil
	case b.covered(true) && b.covered(false):
		if b.publications == 0 {
			b.publications = 1
			b.paramsOut = smb1Materialize(b.params, b.totalP)
			b.dataOut = smb1Materialize(b.data, b.totalD)
			return "complete", nil
		}
		return "retransmit", nil
	case pStatus == "retransmit" && dStatus == "retransmit":
		return "retransmit", nil
	default:
		return "incomplete", nil
	}
}

func (b *smb1Trans2Buffer) add(param bool, at int, chunk []byte) (string, error) {
	total := b.totalD
	spans := b.data
	if param {
		total = b.totalP
		spans = b.params
	}
	if len(chunk) == 0 {
		if b.covered(param) {
			return "retransmit", nil
		}
		if smb1SpanHole(spans, total) {
			return "hole", nil
		}
		return "incomplete", nil
	}
	if at < 0 || at > total || len(chunk) > total-at {
		return "", fmt.Errorf("CIFS TRANSACTION2: displacement is outside the total")
	}
	for _, span := range spans {
		lo, hi := at, at+len(chunk)
		if span.at > lo {
			lo = span.at
		}
		end := span.at + len(span.b)
		if end < hi {
			hi = end
		}
		if lo < hi && !bytes.Equal(span.b[lo-span.at:hi-span.at], chunk[lo-at:hi-at]) {
			return "overlap conflict", nil
		}
	}
	if smb1RangeCovered(spans, at, at+len(chunk)) {
		if smb1SpanHole(spans, total) {
			return "hole", nil
		}
		return "retransmit", nil
	}
	spans = append(spans, smb1Trans2Span{at: at, b: bytes.Clone(chunk)})
	if param {
		b.params = spans
	} else {
		b.data = spans
	}
	if smb1SpanHole(spans, total) {
		return "hole", nil
	}
	if smb1RangeCovered(spans, 0, total) {
		return "complete", nil
	}
	return "incomplete", nil
}

func (b *smb1Trans2Buffer) covered(param bool) bool {
	if param {
		return smb1RangeCovered(b.params, 0, b.totalP)
	}
	return smb1RangeCovered(b.data, 0, b.totalD)
}

func smb1SpanHole(spans []smb1Trans2Span, total int) bool {
	if total == 0 || len(spans) == 0 {
		return false
	}
	ordered := append([]smb1Trans2Span(nil), spans...)
	for i := 1; i < len(ordered); i++ {
		item := ordered[i]
		j := i
		for j > 0 && ordered[j-1].at > item.at {
			ordered[j] = ordered[j-1]
			j--
		}
		ordered[j] = item
	}
	if ordered[0].at > 0 {
		return true
	}
	end := ordered[0].at + len(ordered[0].b)
	for _, span := range ordered[1:] {
		if span.at > end {
			return true
		}
		if span.at+len(span.b) > end {
			end = span.at + len(span.b)
		}
	}
	return false
}

func smb1RangeCovered(spans []smb1Trans2Span, start, end int) bool {
	if start >= end {
		return true
	}
	for at := start; at < end; {
		next := at
		for _, span := range spans {
			if span.at <= at && span.at+len(span.b) > next {
				next = span.at + len(span.b)
			}
		}
		if next == at {
			return false
		}
		at = next
	}
	return true
}

func smb1Materialize(spans []smb1Trans2Span, total int) []byte {
	out := make([]byte, total)
	for _, span := range spans {
		copy(out[span.at:], span.b)
	}
	return out
}

func smb1CloneSpans(spans []smb1Trans2Span) []smb1Trans2Span {
	out := make([]smb1Trans2Span, len(spans))
	for i, span := range spans {
		out[i] = smb1Trans2Span{at: span.at, b: bytes.Clone(span.b)}
	}
	return out
}

func smb1MapUint16(value any) uint16 {
	switch n := value.(type) {
	case uint16:
		return n
	case int:
		return uint16(n)
	default:
		return 0
	}
}

func (f *binFlow) admitSMB1Trans2Secondary(tx *smb1LegacyTxn, fields map[string]any) error {
	ledger := f.smb2.smb1Legacy
	pending, ok := ledger.pending[tx.key]
	if !ok || pending.trans2 == nil {
		fields["Association Status"] = "secondary request has no observed TRANSACTION2 primary"
		return nil
	}
	body, _ := fields["Body"].(map[string]any)
	status, err := pending.trans2.admit(body)
	if err != nil {
		return protocolError(ErrMalformedMessage, "%s", err)
	}
	fields["Association Status"] = "secondary slice of the observed TRANSACTION2 request"
	fields["Reassembly Status"] = status
	switch status {
	case "complete":
		fields["Reassembly Publication"] = "first complete observation"
		f.publishSMB1Trans2(fields, pending)
	case "retransmit":
		fields["Reassembly Publication"] = "retransmit; logical buffer unchanged"
		f.publishSMB1Trans2(fields, pending)
	}
	ledger.pending[tx.key] = pending
	return nil
}

func (f *binFlow) publishSMB1Trans2(fields map[string]any, pending smb1OperationPending) {
	buf := pending.trans2
	fields["Reassembled Parameters"] = bytes.Clone(buf.paramsOut)
	fields["Reassembled Data"] = bytes.Clone(buf.dataOut)
	fields["Reassembly Context"] = "concatenated from observed displacements; no single PDU range"
	if pending.setup == 5 && buf.covered(true) {
		if name, ok := stream_parser.DecodeCIFSQueryPathParameters(buf.paramsOut, buf.unicode); ok {
			fields["Reassembled File Name"] = name
		}
	}
}
