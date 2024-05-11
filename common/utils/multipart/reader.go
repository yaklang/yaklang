// rewrite multipart reader for compatibility with invalid boundary
package multipart

import (
	"bufio"
	"bytes"
	"io"
	"mime"
	"net/textproto"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	peekBufferSize = 4096

	FINDING_BOUNDARY     = 0
	PARSING_BLOCK_HEADER = iota
	PARSING_BLOCK_BODY
	FINISHED
)

var (
	COLON              = []byte(":")
	CRLF               = []byte("\r\n")
	LF                 = []byte("\n")
	BoundaryStartOrEnd = []byte("--")

	ErrInvalidBoundary = utils.Error("multipart: invalid boundary")

	emptyParams = make(map[string]string)
)

type Reader struct {
	bufReader    *bufio.Reader
	dashBoundary []byte
	partsRead    int
}

func NewReaderWithString(s string) *Reader {
	return NewReader(bufio.NewReader(bytes.NewBufferString(s)))
}

func NewReader(r io.Reader) *Reader {
	return &Reader{
		bufReader: bufio.NewReaderSize(r, peekBufferSize),
	}
}

func (r *Reader) Boundary() []byte {
	return r.dashBoundary
}

func (r *Reader) PartsRead() int {
	return r.partsRead
}

func (r *Reader) NextRawPart() (*Part, error) {
	return r.NextPart()
}

func (r *Reader) NextPart() (*Part, error) {
	var (
		currentPart  *Part
		dashBoundary []byte // "--boundary"
		appendNL     []byte
		state        = FINDING_BOUNDARY
		headerBuffer = new(bytes.Buffer)
		blockBuffer  = new(bytes.Buffer)
		isFirstLine  = true
	)

	if r.partsRead > 0 {
		state = PARSING_BLOCK_HEADER
		dashBoundary = r.dashBoundary
	}

	for {
		line, err := r.bufReader.ReadBytes('\n')
		trimed := bytes.TrimRightFunc(line, unicode.IsSpace)

		if err != nil && len(trimed) == 0 {
			if err != io.EOF {
				return nil, utils.Wrap(err, "multipart: NextPart")
			} else {
				if currentPart == nil {
					return nil, io.EOF
				}
				break
			}
		}

		// when FINDING_BOUNDARY skip empty line
		// first line should not empty line
		if len(trimed) == 0 && (state == FINDING_BOUNDARY || isFirstLine) {
			continue
		}
		isFirstLine = false
		isEnd := bytes.Equal(trimed, dashBoundary) || bytes.Equal(trimed, BytesJoinSize(len(dashBoundary)+2, dashBoundary, BoundaryStartOrEnd))
		if isEnd {
			break
		}

		switch state {
		case FINDING_BOUNDARY:
			if !bytes.HasPrefix(trimed, BoundaryStartOrEnd) {
				return nil, ErrInvalidBoundary
			}
			dashBoundary = bytes.Clone(trimed)
			r.dashBoundary = dashBoundary

			state = PARSING_BLOCK_HEADER
		case PARSING_BLOCK_HEADER:
			if currentPart == nil {
				currentPart = newPart(blockBuffer, headerBuffer)
			}

			if len(trimed) == 0 {
				state = PARSING_BLOCK_BODY
				continue
			}

			headerBuffer.Write(line)
			if bytes.Contains(trimed, COLON) {
				k, v, ok := strings.Cut(string(trimed), ":")
				if ok {
					currentPart.Header[textproto.CanonicalMIMEHeaderKey(k)] = append(currentPart.Header[textproto.CanonicalMIMEHeaderKey(k)], strings.TrimSpace(v))
				}
			}
		case PARSING_BLOCK_BODY:
			if appendNL != nil {
				blockBuffer.Write(appendNL)
				appendNL = nil
			}

			if !currentPart.hasBody && len(line) > 0 {
				currentPart.hasBody = true
			}
			blockBuffer.Write(trimed)
			if bytes.HasSuffix(line, CRLF) {
				appendNL = CRLF
			} else if bytes.HasSuffix(line, LF) {
				appendNL = LF
			}

			// case FINISHED:
			// 	break LOOP
		}
	}

	r.partsRead++
	return currentPart, nil
}

var _ = io.Reader((*Part)(nil))

// Part represents a single part of a multipart body.

type Part struct {
	Header       textproto.MIMEHeader
	headerReader io.Reader
	bodyReader   io.Reader

	disposition       string
	dispositionParams map[string]string
	hasBody           bool
}

func newPart(bodyReader, headerReader io.Reader) *Part {
	p := &Part{
		bodyReader:   bodyReader,
		headerReader: headerReader,
		Header:       make(textproto.MIMEHeader),
	}
	return p
}

func (p *Part) GetHeader(key string, canonicals ...bool) string {
	canonical := false
	if len(canonicals) > 0 {
		canonical = canonicals[0]
	}
	if canonical {
		key = textproto.CanonicalMIMEHeaderKey(key)
	}

	values, ok := p.Header[key]
	if !ok || len(values) == 0 {
		return ""
	}
	return values[0]
}

func (p *Part) ReadRawHeader() ([]byte, error) {
	if p.headerReader == nil {
		return nil, io.EOF
	}
	return io.ReadAll(p.headerReader)
}

func (p *Part) HasBody() bool {
	return p.hasBody
}

func (p *Part) Read(data []byte) (n int, err error) {
	if p.bodyReader == nil {
		return 0, io.EOF
	}

	return p.bodyReader.Read(data)
}

func (p *Part) FileName(stricts ...bool) string {
	strict := false
	if len(stricts) > 0 {
		strict = stricts[0]
	}
	if p.dispositionParams == nil {
		p.parseContentDisposition()
	}
	filename := p.dispositionParams["filename"]
	if filename == "" {
		return ""
	}
	if strict {
		// RFC 7578, Section 4.2 requires that if a filename is provided, the
		// directory path information must not be used.
		return filepath.Base(filename)
	} else {
		return filename
	}
}

func (p *Part) FormName(stricts ...bool) string {
	strict := false
	if len(stricts) > 0 {
		strict = stricts[0]
	}
	// See https://tools.ietf.org/html/rfc2183 section 2 for EBNF
	// of Content-Disposition value format.
	if p.dispositionParams == nil {
		p.parseContentDisposition()
	}
	if strict && p.disposition != "form-data" {
		return ""
	}
	return p.dispositionParams["name"]
}

func (p *Part) parseContentDisposition() {
	v := p.GetHeader("Content-Disposition")
	var err error
	p.disposition, p.dispositionParams, err = mime.ParseMediaType(v)
	if err != nil {
		p.dispositionParams = emptyParams
	}
}
