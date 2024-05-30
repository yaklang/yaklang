// rewrite multipart reader for compatibility with invalid boundary
package multipart

import (
	"bufio"
	"bytes"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"mime"
	"net/textproto"
	"path/filepath"
	"strings"
)

const (
	peekBufferSize = 4096

	FINDING_BOUNDARY     = 0
	BOUNDARY_MATCHED     = 1
	PARSING_BLOCK_HEADER = iota
	PARSING_BLOCK_BODY
	PART_FINISHED
	FINISHED
)

var (
	CRLF = []byte("\r\n")
	LF   = []byte("\n")

	ErrInvalidBoundary = utils.Error("multipart: invalid boundary")

	emptyParams = make(map[string]string)
)

type Reader struct {
	bufReader *bufio.Reader
	boundary  string
	finished  bool
	partsRead int
}

func NewReaderWithString(s string) *Reader {
	return NewReader(bufio.NewReader(bytes.NewBufferString(s)))
}

func NewReader(r io.Reader) *Reader {
	return &Reader{
		bufReader: bufio.NewReaderSize(r, peekBufferSize),
	}
}

func (r *Reader) Boundary() string {
	return r.boundary
}

func (r *Reader) PartsRead() int {
	return r.partsRead
}

func (r *Reader) NextRawPart() (*Part, error) {
	return r.NextPart()
}

func (r *Reader) NextPart() (*Part, error) {
	if r.finished {
		return nil, io.EOF
	}
	var (
		currentPart  *Part
		state        = FINDING_BOUNDARY
		headerBuffer = new(bytes.Buffer)
		bodyBuffer   = new(bytes.Buffer)
		bareBuffer   = new(bytes.Buffer)

		lastBodyDivider []byte
	)

	if len(r.boundary) > 0 {
		state = BOUNDARY_MATCHED
		if r.partsRead > 0 {
			state = PARSING_BLOCK_HEADER
		}
	} else {
		state = FINDING_BOUNDARY
	}

	for {
		line, err := r.bufReader.ReadBytes('\n')
		var (
			trimed             []byte
			isEmptyLine        bool
			splitCRLF, splitLF bool
		)
		if len(line) > 1 && line[len(line)-1] == '\n' {
			lastIndex := len(line) - 1
			splitCRLF = line[lastIndex-1] == '\r'
			splitLF = !splitCRLF
			isEmptyLine = len(line) == 2
			if splitCRLF {
				trimed = line[:lastIndex-1]
			} else {
				trimed = line[:lastIndex]
			}
		} else if len(line) > 1 && line[len(line)-1] != '\n' {
			trimed = line
			splitCRLF = false
			splitLF = false
		} else if len(line) == 1 {
			splitLF = true
			splitCRLF = false
			isEmptyLine = true
		} else {
			isEmptyLine = true
		}

		if isEmptyLine && err != nil {
			if currentPart == nil {
				return nil, err
			}
			r.finished = true
			return currentPart, nil
		}

	StateTransfer:
		switch state {
		case FINDING_BOUNDARY:
			lineBoundary := string(trimed)
			if strings.HasPrefix(lineBoundary, "--") {
				r.boundary = lineBoundary[2:]
			} else {
				return nil, utils.Error("invalid boundary, boundary should start with '--'")
			}
			state = PARSING_BLOCK_HEADER
		case BOUNDARY_MATCHED:
			lineBoundary := string(trimed)
			if lineBoundary == "--"+r.boundary {
				state = PARSING_BLOCK_HEADER
				continue
			}
		case PARSING_BLOCK_HEADER:
			if currentPart == nil {
				currentPart = newPart(bodyBuffer, headerBuffer, bareBuffer)
				currentPart.SetNoEmptyLineDivider(true)
				currentPart.SetNoBody(true)
			}
			if isEmptyLine {
				state = PARSING_BLOCK_BODY
				currentPart.SetNoEmptyLineDivider(false)
				lastBodyDivider = nil
				continue
			} else if ret := len(trimed); ret == len(r.boundary)+2 || ret == len(r.boundary)+4 {
				mayBoundary := string(trimed)
				if mayBoundary == "--"+r.boundary+"--" {
					state = FINISHED
					goto StateTransfer
				} else if mayBoundary == "--"+r.boundary {
					state = PART_FINISHED
					goto StateTransfer
				}
			}

			headerBuffer.Write(trimed)
			headerBuffer.Write(CRLF)
			bareBuffer.Write(line)

			k, v, ok := strings.Cut(string(trimed), ":")
			if ok {
				if strings.HasPrefix(v, " ") {
					v = v[1:]
				}
				currentPart.Header[textproto.CanonicalMIMEHeaderKey(k)] = append(currentPart.Header[textproto.CanonicalMIMEHeaderKey(k)], v)
			}
		case PARSING_BLOCK_BODY:
			ret := len(trimed)
			// check if it's boundary
			if ret == len(r.boundary)+2 || ret == len(r.boundary)+4 {
				mayBoundary := string(trimed)
				if mayBoundary == "--"+r.boundary+"--" {
					state = FINISHED
					goto StateTransfer
				} else if mayBoundary == "--"+r.boundary {
					state = PART_FINISHED
					goto StateTransfer
				}
			}
			if currentPart != nil {
				currentPart.SetNoBody(false)
			}
			if lastBodyDivider != nil {
				bodyBuffer.Write(lastBodyDivider)
			}
			bodyBuffer.Write(trimed)
			if splitLF {
				lastBodyDivider = LF
			} else if splitCRLF {
				lastBodyDivider = CRLF
			}
		case PART_FINISHED:
			r.finished = false
			r.partsRead++
			return currentPart, nil
		case FINISHED:
			r.finished = true
			return currentPart, nil
		default:
			return nil, utils.Error("invalid state")
		}
	}
}

var _ = io.Reader((*Part)(nil))

// Part represents a single part of a multipart body.

type Part struct {
	Header textproto.MIMEHeader

	headerReader       io.Reader
	bareHeaderReader   io.Reader
	noEmptyLineDivider bool

	noBody     bool
	bodyReader io.Reader

	disposition       string
	dispositionParams map[string]string
}

func (p *Part) SetNoEmptyLineDivider(b bool) {
	p.noEmptyLineDivider = b
}

func (p *Part) NoEmptyLineDivider() bool {
	return p.noEmptyLineDivider
}

func (p *Part) SetNoBody(noBody bool) {
	p.noBody = noBody
}

func (p *Part) NoBody() bool {
	return p.noBody
}

func newPart(bodyReader, headerReader, bareHeaderReader io.Reader) *Part {
	p := &Part{
		bodyReader:       bodyReader,
		headerReader:     headerReader,
		bareHeaderReader: bareHeaderReader,
		Header:           make(textproto.MIMEHeader),
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
