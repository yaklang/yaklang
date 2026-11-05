package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func observedHTTPResponseHeader(e *pcaputil.ProtocolEvent) *httpEvidence {
	if e == nil || e.Protocol != "http" || e.Transport != "tcp" || e.Profile != "http-response-header-observation" ||
		e.Admission != "complete-header-without-request-method" || e.Completeness != "headers" ||
		!(e.Status == "context-required" && e.ExpertCode == string(pcaputil.ErrContextRequired) || e.Status == "limited" && e.ExpertCode == string(pcaputil.ErrResourceExceeded)) ||
		e.ResponseTo != 0 || e.TransactionID != 0 || e.SourceBytes.CompletedIn != nil || e.Length != len(e.Raw) ||
		(e.SourceBytes.Kind != "reassembled" && e.SourceBytes.Kind != "decrypted") ||
		e.SourceBytes.SHA256 != fmt.Sprintf("%x", sha256.Sum256(e.Raw)) ||
		e.Session["Request Context Available"] != false || e.Session["Body Boundary Known"] != false ||
		e.Session["HTTP Observation Scope"] != "validated response header; request method unavailable" {
		return nil
	}
	if _, present := e.Session["Method"]; present {
		return nil
	}
	retained, retainedOK := detailRangeOffset(e.Session["Retained Stream Bytes"])
	unframed, unframedOK := detailRangeOffset(e.Session["Unframed Stream Bytes"])
	if !retainedOK || !unframedOK || retained < len(e.Raw) || retained-len(e.Raw) != unframed {
		return nil
	}
	h := inspectHTTPBounded(e.Raw, 32<<10)
	if h == nil || h.status == 0 || h.length != len(e.Raw) ||
		len(h.header.Values("Content-Length")) > 0 && len(h.header.Values("Transfer-Encoding")) > 0 {
		return nil
	}
	return h
}

func httpUnframedPrefix(e *pcaputil.ProtocolEvent) ([]byte, map[string]any) {
	m, ok := e.Session["Unframed Stream Observation"].(map[string]any)
	if !ok {
		return nil, nil
	}
	prefix, typed := m["Unframed Stream Prefix"].([]byte)
	prefixBytes, prefixOK := detailRangeOffset(m["Prefix Bytes"])
	observed, observedOK := detailRangeOffset(m["Observed Bytes"])
	total, totalOK := detailRangeOffset(e.Session["Unframed Stream Bytes"])
	offset, offsetOK := gpeHeaderInteger(m["Offset"], ^uint64(0))
	if !typed || len(prefix) == 0 || len(prefix) > decoderInputLimit || !prefixOK || prefixBytes != len(prefix) ||
		!observedOK || !totalOK || observed != total || observed < prefixBytes ||
		!offsetOK || offset < e.Offset || offset-e.Offset != uint64(len(e.Raw)) ||
		m["Source Kind"] != e.SourceBytes.Kind || m["Preview Limited"] != (prefixBytes < observed) ||
		m["Observation Scope"] != "ordered bytes after observed header; body and next-message boundaries unknown" ||
		m["Prefix SHA256"] != fmt.Sprintf("%x", sha256.Sum256(prefix)) {
		return nil, nil
	}
	wholeHash, hashOK := m["Observed SHA256"].(string)
	decodedHash, hashErr := hex.DecodeString(wholeHash)
	if !hashOK || hashErr != nil || len(decodedHash) != sha256.Size ||
		observed == prefixBytes && wholeHash != m["Prefix SHA256"] {
		return nil, nil
	}
	return prefix, m
}

func observedHTTPUnframedContext(e *pcaputil.ProtocolEvent) string {
	if e == nil || e.Protocol != "http" || e.Transport != "tcp" || e.Profile != "http-unframed-stream-observation" ||
		e.Admission != "after-complete-header-without-request-method" || e.Completeness != "context-required" ||
		e.Status != "context-required" || e.ExpertCode != string(pcaputil.ErrContextRequired) ||
		e.SourceBytes.CompletedIn != nil || e.ResponseTo != 0 || e.TransactionID != 0 || e.Length < len(e.Raw) || len(e.Raw) == 0 ||
		(e.SourceBytes.Kind != "reassembled" && e.SourceBytes.Kind != "decrypted") ||
		e.SourceBytes.SHA256 != fmt.Sprintf("%x", sha256.Sum256(e.Raw)) ||
		e.Session["Body Boundary Known"] != false || e.Session["Request Context Available"] != false ||
		e.Session["Observation Scope"] != "ordered bytes after a response header with no request method; message boundary unknown" ||
		e.Session["Preview Limited"] != (e.Length > len(e.Raw)) {
		return ""
	}
	if _, present := e.Session["Method"]; present {
		return ""
	}
	version, ok := e.Session["Observed Header Protocol"].(string)
	if !ok || !httpVersion(version) {
		return ""
	}
	parent, parentOK := gpeHeaderInteger(e.Session["Observed Header PDU"], ^uint64(0))
	observed, observedOK := detailRangeOffset(e.Session["Observed Bytes"])
	if !parentOK || parent == 0 || !observedOK || observed != e.Length {
		return ""
	}
	for _, key := range []string{"Observed Header SHA256", "Observed SHA256"} {
		text, ok := e.Session[key].(string)
		b, err := hex.DecodeString(text)
		if !ok || err != nil || len(b) != sha256.Size {
			return ""
		}
	}
	if e.Length == len(e.Raw) && e.Session["Observed SHA256"] != e.SourceBytes.SHA256 {
		return ""
	}
	return version
}

func appendHTTPUnframedContext(out *[]field, sources *[]detailSource, e *pcaputil.ProtocolEvent) {
	name := fmt.Sprintf("Unframed HTTP stream bytes · %d observed bytes · %s", e.Length, e.SourceBytes.Kind)
	if e.Length > len(e.Raw) {
		name = fmt.Sprintf("Unframed HTTP stream preview · %d/%d observed bytes · %s", len(e.Raw), e.Length, e.SourceBytes.Kind)
	}
	*sources = append(*sources, detailSource{Name: name, Data: bytes.Clone(e.Raw)})
	*out = append(*out,
		field{text: fmt.Sprintf("HTTP response context · unframed observation #%d", e.ID), branch: true, source: len(*sources), end: len(e.Raw), hasRange: true},
		field{depth: 1, text: "Body boundary: unknown · request method unavailable"},
		field{depth: 1, text: fmt.Sprintf("Observed header PDU: #%d · %s · context only", number(e.Session["Observed Header PDU"]), textValue(e.Session["Observed Header Protocol"]))},
		field{depth: 1, text: fmt.Sprintf("Stream offset: %d; preview: %d/%d bytes", e.Offset, len(e.Raw), e.Length)},
		field{depth: 1, text: "No complete HTTP message or tunnel boundary is inferred"},
		field{depth: 1, text: "Observed SHA256: " + textValue(e.Session["Observed SHA256"])},
		field{depth: 1, text: "Prefix SHA256: " + e.SourceBytes.SHA256},
	)
}

func isHTTPObservationProfile(e *pcaputil.ProtocolEvent) bool {
	return e != nil && (e.Profile == "http-response-header-observation" || e.Profile == "http-unframed-stream-observation" || e.Profile == "http-incomplete-message-header-observation")
}

func appendUnqualifiedHTTPObservation(out *[]field, sources *[]detailSource, e *pcaputil.ProtocolEvent) {
	root := field{text: fmt.Sprintf("TCP stream observation #%d · HTTP context unavailable", e.ID), branch: true}
	if len(e.Raw) > 0 {
		*sources = append(*sources, detailSource{Name: "Unconfirmed TCP stream bytes · observation", Data: bytes.Clone(e.Raw)})
		root.source, root.end, root.hasRange = len(*sources), len(e.Raw), true
	}
	*out = append(*out, root, field{depth: 1, text: "No complete HTTP message or body boundary is inferred"})
	if e.Error != "" {
		*out = append(*out, field{depth: 1, text: "Diagnostic: " + safeText(e.Error)})
	}
}

func appendHTTPHeaderObservation(out *[]field, sources *[]detailSource, e *pcaputil.ProtocolEvent, h *httpEvidence) {
	label := strings.SplitN(h.line, " ", 2)[0]
	*sources = append(*sources, detailSource{Name: label + " response header · " + e.SourceBytes.Kind + " · header bytes", Data: bytes.Clone(e.Raw)})
	headerSource := len(*sources)
	*out = append(*out,
		field{text: fmt.Sprintf("%s response header · observation #%d · %s", label, e.ID, e.SourceBytes.Kind), branch: true, source: headerSource, end: len(e.Raw), hasRange: true},
		field{depth: 1, text: "Body boundary: unknown · request method unavailable"},
		field{depth: 1, text: fmt.Sprintf("Retained stream: %v bytes; unframed: %v bytes", e.Session["Retained Stream Bytes"], e.Session["Unframed Stream Bytes"])},
		field{depth: 1, text: "Header", branch: true, source: headerSource, end: len(e.Raw), hasRange: true},
	)
	// Preserve the original order and duplicates. A header's source remains
	// its actual complete line, never the range of a merged MIME-map value.
	pos, count := 0, 0
	for pos < len(e.Raw)-2 {
		n := bytes.Index(e.Raw[pos:], []byte("\r\n"))
		if n <= 0 {
			break
		}
		if count >= 1024 {
			*out = append(*out, field{depth: 2, text: "Header view limited; complete header bytes remain available"})
			break
		}
		*out = append(*out, field{depth: 2, text: safeText(string(e.Raw[pos : pos+n])), source: headerSource, start: pos, end: pos + n, hasRange: true})
		pos, count = pos+n+2, count+1
	}
	if prefix, m := httpUnframedPrefix(e); m != nil {
		observed := number(m["Observed Bytes"])
		name := fmt.Sprintf("Unframed HTTP stream bytes · %d observed bytes · %s", observed, e.SourceBytes.Kind)
		if uint64(len(prefix)) < observed {
			name = fmt.Sprintf("Unframed HTTP stream preview · %d/%d observed bytes · %s", len(prefix), observed, e.SourceBytes.Kind)
		}
		*sources = append(*sources, detailSource{Name: name, Data: bytes.Clone(prefix)})
		*out = append(*out,
			field{depth: 1, text: "Unframed stream bytes · select for bytes", branch: true, source: len(*sources), end: len(prefix), hasRange: true},
			field{depth: 2, text: "Scope: body and next-message boundaries unknown"},
			field{depth: 2, text: fmt.Sprintf("Stream offset: %v; preview: %d/%d bytes", m["Offset"], len(prefix), observed)},
			field{depth: 2, text: "Observed SHA256: " + textValue(m["Observed SHA256"])},
			field{depth: 2, text: "Prefix SHA256: " + textValue(m["Prefix SHA256"])},
		)
	} else if number(e.Session["Unframed Stream Bytes"]) != 0 {
		reason := textValue(e.Session["Unframed Preview Unavailable"])
		if reason == "" {
			reason = "observation source inconsistent or unavailable"
		}
		*out = append(*out, field{depth: 1, text: "Unframed preview unavailable: " + reason})
	}
	*out = append(*out,
		field{depth: 1, text: "Context", branch: true},
		field{depth: 2, text: "Request method: unavailable; response association: unavailable"},
		field{depth: 2, text: "No complete HTTP message or tunnel boundary is inferred"},
		field{depth: 2, text: "Status: " + e.Status + " · headers"},
	)
}
