package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func observedHTTPIncompleteHeader(e *pcaputil.ProtocolEvent) *httpEvidence {
	if e == nil || e.Protocol != "http" || e.Transport != "tcp" || e.Profile != "http-incomplete-message-header-observation" ||
		e.Admission != "validated-header-without-complete-message" || e.Completeness != "headers" || e.Status != "incomplete" ||
		e.ExpertCode != string(pcaputil.ErrNeedMore) || e.ResponseTo != 0 || e.TransactionID != 0 || e.SourceBytes.CompletedIn != nil ||
		e.Length != len(e.Raw) || (e.SourceBytes.Kind != "reassembled" && e.SourceBytes.Kind != "decrypted") ||
		e.SourceBytes.SHA256 != fmt.Sprintf("%x", sha256.Sum256(e.Raw)) || e.Session["Message Complete"] != false ||
		e.Session["HTTP Observation Scope"] != "validated response header; exact message boundary not reached" {
		return nil
	}
	header, ok := detailRangeOffset(e.Session["Header Bytes"])
	retained, retainedOK := detailRangeOffset(e.Session["Retained Stream Bytes"])
	after, afterOK := detailRangeOffset(e.Session["Bytes After Header"])
	if !ok || header <= 0 || header > len(e.Raw) || header > 32<<10 || !retainedOK || retained != len(e.Raw) ||
		!afterOK || after != len(e.Raw)-header || e.Session["Observed Header SHA256"] != fmt.Sprintf("%x", sha256.Sum256(e.Raw[:header])) {
		return nil
	}
	h := inspectHTTPBounded(e.Raw[:header], 32<<10)
	if h == nil || h.status == 0 || h.length != header ||
		len(h.header.Values("Content-Length")) > 0 && len(h.header.Values("Transfer-Encoding")) > 0 {
		return nil
	}
	return h
}

func appendHTTPIncompleteHeader(out *[]field, sources *[]detailSource, e *pcaputil.ProtocolEvent, h *httpEvidence) {
	label := strings.SplitN(h.line, " ", 2)[0]
	*sources = append(*sources, detailSource{Name: label + " incomplete message · " + e.SourceBytes.Kind + " · observed bytes", Data: bytes.Clone(e.Raw)})
	source := len(*sources)
	*out = append(*out,
		field{text: fmt.Sprintf("%s response · incomplete observation #%d · %s", label, e.ID, e.SourceBytes.Kind), branch: true, source: source, end: len(e.Raw), hasRange: true},
		field{depth: 1, text: "Message incomplete · exact body boundary not reached"},
		field{depth: 1, text: "Header observed · no complete-message or response association inferred"},
	)
	if e.SourceBytes.Kind == "decrypted" && e.Session["TLS Plaintext Gap"] == true {
		for _, key := range []string{"TLS Plaintext Gap", "TLS Gap Cause PDU", "TLS Gap Direction", "TLS Gap Version"} {
			if value, ok := e.Session[key]; ok {
				*out = append(*out, field{depth: 1, text: key + ": " + safeText(formatFieldValue(key, value))})
			}
		}
	}
	*out = append(*out, field{depth: 1, text: "Header", branch: true, source: source, end: h.length, hasRange: true})
	pos := 0
	for count := 0; pos < h.length-2 && count < 1024; count++ {
		n := bytes.Index(e.Raw[pos:h.length], []byte("\r\n"))
		if n <= 0 {
			break
		}
		*out = append(*out, field{depth: 2, text: safeText(string(e.Raw[pos : pos+n])), source: source, start: pos, end: pos + n, hasRange: true})
		pos += n + 2
	}
	if pos < h.length-2 {
		*out = append(*out, field{depth: 2, text: "Header view limited; complete header bytes remain available"})
	}
	if len(e.Raw) > h.length {
		*out = append(*out, field{depth: 1, text: fmt.Sprintf("Observed bytes after header: %d · incomplete message", len(e.Raw)-h.length), source: source, start: h.length, end: len(e.Raw), hasRange: true})
	}
	*out = append(*out, field{depth: 1, text: "Diagnostic: " + safeText(e.Summary)})
}
