package yaklib

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type HTTPPacketFile struct {
	Part, Path string
	Bytes      int64
}

func (f *HTTPPacketFile) Dump() string {
	return fmt.Sprintf("%s_file=%s bytes=%d\nUse grep or read_file on this file to inspect the complete raw HTTP packet.", f.Part, f.Path, f.Bytes)
}

// ExportPackets streams complete packets, including large request/response and
// multipart sidecars. Each file is newly created with 0600 permissions.
func (p *HTTPHistoryItem) ExportPackets(directory, part string) ([]*HTTPPacketFile, error) {
	if part != "both" && part != "request" && part != "response" {
		return nil, fmt.Errorf("packet must be request, response or both")
	}
	if directory == "" {
		directory = os.TempDir()
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	files := []*HTTPPacketFile{}
	complete := false
	defer func() {
		if !complete {
			for _, f := range files {
				_ = os.Remove(f.Path)
			}
		}
	}()
	for _, which := range []string{"request", "response"} {
		if part != "both" && part != which {
			continue
		}
		if err := p.config.ctx.Err(); err != nil {
			return nil, err
		}
		reader, closeReader, err := p.openPacket(which)
		if err != nil {
			return nil, err
		}
		file, err := os.CreateTemp(directory, fmt.Sprintf("http-flow-%d-%s-*.http", p.ID, which))
		if err != nil {
			closeReader()
			return nil, err
		}
		n, err := io.Copy(file, &contextPacketReader{ctx: p.config.ctx, r: reader})
		closeReader()
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(file.Name())
			return nil, err
		}
		path, err := filepath.Abs(file.Name())
		if err != nil {
			_ = os.Remove(file.Name())
			return nil, err
		}
		files = append(files, &HTTPPacketFile{Part: which, Path: path, Bytes: n})
	}
	complete = true
	return files, nil
}

type contextPacketReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextPacketReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
func regularPacketFile(path string) (*os.File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("HTTP packet resource is not a regular file: %s", path)
	}
	return os.Open(path)
}
func (p *HTTPHistoryItem) openPacket(part string) (io.Reader, func(), error) {
	header, body := p.TooLargeResponseHeaderFile, p.TooLargeResponseBodyFile
	if part == "request" {
		header, body = p.TooLargeRequestHeaderFile, p.TooLargeRequestBodyFile
	}
	if body != "" {
		if header == "" {
			return nil, nil, fmt.Errorf("%s sidecar header is missing; cannot export a complete packet", part)
		}
		h, err := regularPacketFile(header)
		if err != nil {
			return nil, nil, err
		}
		flow := p.HTTPFlow
		if part == "request" && p.IsTooLargeRequest && flow.Request == "" {
			// A bounded projection may omit even the multipart skeleton. Read at most
			// 8 MiB of the stored representation, never the spilled part bodies.
			c := p.config
			db, closeDB, err := historyProjectDatabase(&c)
			if err != nil {
				h.Close()
				return nil, nil, err
			}
			decoded := &unquotedPacketReader{source: bufio.NewReader(&sqlitePacketReader{ctx: c.ctx, db: db, id: int64(p.ID), column: "request", offset: 1})}
			skeleton, err := io.ReadAll(io.LimitReader(decoded, (8<<20)+1))
			closeDB()
			if err != nil || len(skeleton) > 8<<20 {
				h.Close()
				if err == nil {
					err = fmt.Errorf("multipart skeleton exceeds 8 MiB")
				}
				return nil, nil, err
			}
			copyFlow := *flow
			copyFlow.SetRequest(string(skeleton))
			flow = &copyFlow
		}
		if part == "request" && yakit.FlowIsMultipartSpill(flow) {
			b, err := yakit.RebuildFlowMultipartBody(flow)
			if err != nil {
				h.Close()
				return nil, nil, err
			}
			return io.MultiReader(h, b), func() {
				_ = h.Close()
				if closer, ok := b.(io.Closer); ok {
					_ = closer.Close()
				}
			}, nil
		}
		b, err := regularPacketFile(body)
		if err != nil {
			h.Close()
			return nil, nil, err
		}
		return io.MultiReader(h, b), func() { _ = h.Close(); _ = b.Close() }, nil
	}
	if (part == "request" && p.IsTooLargeRequest) || (part == "response" && (p.IsTooLargeResponse || p.IsReadTooSlowResponse)) {
		return nil, nil, fmt.Errorf("%s sidecar body is missing; cannot export a complete packet", part)
	}
	c := p.config
	db, closeDB, err := historyProjectDatabase(&c)
	if err != nil {
		return nil, nil, err
	}
	raw := &sqlitePacketReader{ctx: c.ctx, db: db, id: int64(p.ID), column: part, offset: 1}
	decoded := &unquotedPacketReader{source: bufio.NewReader(raw)}
	return decoded, closeDB, nil
}

// Read fixed-size SQLite byte ranges, avoiding a full quoted packet allocation.
type sqlitePacketReader struct {
	ctx    context.Context
	db     *gorm.DB
	id     int64
	column string
	offset int
	chunk  []byte
	done   bool
}

func (r *sqlitePacketReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(r.chunk) == 0 {
		if r.done {
			return 0, io.EOF
		}
		sql := fmt.Sprintf("SELECT substr(CAST(%s AS BLOB),?,65536) FROM http_flows WHERE id=? AND deleted_at IS NULL", r.column)
		if err := r.db.DB().QueryRowContext(r.ctx, sql, r.offset, r.id).Scan(&r.chunk); err != nil {
			return 0, err
		}
		r.offset += len(r.chunk)
		if len(r.chunk) < 65536 {
			r.done = true
		}
		if len(r.chunk) == 0 {
			return 0, io.EOF
		}
	}
	n := copy(p, r.chunk)
	r.chunk = r.chunk[n:]
	return n, nil
}

// HTTPFlow uses strconv.Quote, including Go-only \x/\a/\v escapes. Decode
// incrementally across SQL chunk boundaries without evaluating any fuzztags.
type unquotedPacketReader struct {
	source                *bufio.Reader
	started, quoted, done bool
	pending               []byte
}

func (r *unquotedPacketReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if !r.started {
		r.started = true
		b, err := r.source.ReadByte()
		if err != nil {
			return 0, err
		}
		r.quoted = b == '"'
		if !r.quoted {
			r.pending = []byte{b}
		}
	}
	if !r.quoted {
		n := copy(p, r.pending)
		r.pending = r.pending[n:]
		if n > 0 {
			return n, nil
		}
		return r.source.Read(p)
	}
	n := 0
	for n < len(p) {
		if len(r.pending) > 0 {
			count := copy(p[n:], r.pending)
			r.pending = r.pending[count:]
			n += count
			continue
		}
		if r.done {
			if n > 0 {
				return n, nil
			}
			return 0, io.EOF
		}
		b, err := r.source.ReadByte()
		if err != nil {
			if err == io.EOF {
				err = fmt.Errorf("incomplete quoted HTTP packet")
			}
			return n, err
		}
		if b == '"' {
			if _, err := r.source.Peek(1); err != io.EOF {
				if err == nil {
					err = fmt.Errorf("trailing bytes after quoted HTTP packet")
				}
				return n, err
			}
			r.done = true
			continue
		}
		if b != '\\' {
			p[n] = b
			n++
			continue
		}
		escape, err := r.source.ReadByte()
		if err != nil {
			return n, err
		}
		count := 0
		switch {
		case escape == 'x':
			count = 2
		case escape == 'u':
			count = 4
		case escape == 'U':
			count = 8
		case escape >= '0' && escape <= '7':
			count = 2
		}
		token := []byte{'\\', escape}
		if count > 0 {
			extra := make([]byte, count)
			if _, err := io.ReadFull(r.source, extra); err != nil {
				return n, err
			}
			token = append(token, extra...)
		}
		text, err := strconv.Unquote("\"" + string(token) + "\"")
		if err != nil {
			return n, err
		}
		r.pending = []byte(text)
	}
	return n, nil
}
