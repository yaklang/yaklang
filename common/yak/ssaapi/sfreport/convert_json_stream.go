package sfreport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"sort"

	"github.com/yaklang/yaklang/common/utils"
)

// reportSnapshotBufferSize bounds how much of a report document is held in
// memory while it is written. The document itself is never assembled: on
// javacms-core it is 408MB, and building it with json.MarshalIndent allocated
// about 1.4GB per snapshot (the indented copy plus the marshal's own scratch).
const reportSnapshotBufferSize = 64 << 10

// reportJSONSink streams a Report as the same indented document that
// json.MarshalIndent(r, "", "  ") produces, one rule, risk and file at a time.
//
// The element buffer is reused, so the memory a snapshot needs is bounded by
// the largest single element instead of growing with the document.
type reportJSONSink struct {
	out *bufio.Writer
	buf bytes.Buffer
	enc *json.Encoder
	err error
}

func newReportJSONSink(w io.Writer) *reportJSONSink {
	s := &reportJSONSink{out: bufio.NewWriterSize(w, reportSnapshotBufferSize)}
	s.enc = json.NewEncoder(&s.buf)
	// Every element sits one level below a top-level field ("Rules": [ ... ]),
	// so its continuation lines start with the same four spaces.
	s.enc.SetIndent("    ", "  ")
	return s
}

func (s *reportJSONSink) raw(text string) {
	if s.err != nil {
		return
	}
	_, s.err = s.out.WriteString(text)
}

// value writes one JSON value using the indentation of an element at depth two.
func (s *reportJSONSink) value(v any) {
	if s.err != nil {
		return
	}
	s.buf.Reset()
	if err := s.enc.Encode(v); err != nil {
		s.err = err
		return
	}
	data := s.buf.Bytes()
	// Encoder.Encode terminates the value with a newline; the document
	// separator is written by the caller.
	if n := len(data); n > 0 && data[n-1] == '\n' {
		data = data[:n-1]
	}
	if _, err := s.out.Write(data); err != nil {
		s.err = err
	}
}

// field writes one scalar member, encoded exactly as a full marshal would.
func (s *reportJSONSink) field(name string, v any) {
	if s.err != nil {
		return
	}
	s.raw("  \"" + name + "\": ")
	s.value(v)
	s.raw(",\n")
}

// array writes a slice member one element at a time.
//
// A nil slice marshals to null and an empty slice to [], so the two cases are
// kept distinct: the document must stay identical to a full marshal.
func (s *reportJSONSink) array(name string, count int, nilSlice bool, at func(i int) any) {
	if s.err != nil {
		return
	}
	if count == 0 {
		if nilSlice {
			s.raw("  \"" + name + "\": null")
		} else {
			s.raw("  \"" + name + "\": []")
		}
		return
	}
	s.raw("  \"" + name + "\": [")
	for i := 0; i < count; i++ {
		if i > 0 {
			s.raw(",")
		}
		s.raw("\n    ")
		s.value(at(i))
	}
	s.raw("\n  ]")
}

// riskMap writes the risk map member. encoding/json sorts map keys, so the
// streamed document has to sort them too.
func (s *reportJSONSink) riskMap(name string, risks map[string]*Risk) {
	if s.err != nil {
		return
	}
	if risks == nil {
		s.raw("  \"" + name + "\": null")
		return
	}
	keys := make([]string, 0, len(risks))
	for key := range risks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		s.raw("  \"" + name + "\": {}")
		return
	}
	s.raw("  \"" + name + "\": {")
	for i, key := range keys {
		if i > 0 {
			s.raw(",")
		}
		encoded, err := json.Marshal(key)
		if err != nil {
			s.err = err
			return
		}
		s.raw("\n    ")
		if _, err := s.out.Write(encoded); err != nil {
			s.err = err
			return
		}
		s.raw(": ")
		s.value(risks[key])
	}
	s.raw("\n  }")
}

// writeJSONDocument streams the report in the shape of a full marshal of the
// struct: every exported field in declaration order.
//
// The caller holds the read lock while this runs so the collected slices
// cannot change mid-document.
func (r *Report) writeJSONDocument(w io.Writer) error {
	s := newReportJSONSink(w)
	s.raw("{\n")
	s.field("report_type", r.ReportType)
	s.field("engine_version", r.EngineVersion)
	s.field("report_time", r.ReportTime)
	s.field("program_name", r.ProgramName)
	s.field("program_lang", r.ProgramLang)
	s.field("description", r.Description)
	s.field("repository_url", r.RepositoryURL)
	s.field("file_count", r.FileCount)
	s.field("code_line_count", r.CodeLineCount)
	s.field("scan_start_time", r.ScanStartTime)
	s.field("scan_end_time", r.ScanEndTime)
	s.field("RiskNums", r.RiskNums)
	s.array("Rules", len(r.Rules), r.Rules == nil, func(i int) any { return r.Rules[i] })
	s.raw(",\n")
	s.riskMap("Risks", r.Risks)
	s.raw(",\n")
	s.array("File", len(r.File), r.File == nil, func(i int) any { return r.File[i] })
	s.raw("\n}")
	if s.err != nil {
		return utils.Wrapf(s.err, "stream report document failed")
	}
	return s.out.Flush()
}

// writeJSONSnapshot streams the document while holding the report stable.
//
// It replaces a full json.MarshalIndent: the document on disk is the same, but
// nothing proportional to its size stays in memory.
func (r *Report) writeJSONSnapshot(w io.Writer) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.writeJSONDocument(w)
}
