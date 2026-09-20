// Derived from go-ora v3.0.1 network/basic_session.go; see LICENSE.
package oracleprobe

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const maxField = 64 << 10

// TTC uses length-prefixed integers and CLR/DLC strings. A sticky error keeps
// upstream optional-field reads from accidentally accepting truncated input.
func (s *session) fail(err error) error {
	if s.err == nil {
		s.err = err
	}
	return s.err
}
func (s *session) GetBytes(n int) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	if n < 0 || n > maxField {
		return nil, s.fail(fmt.Errorf("oracle: invalid field length %d", n))
	}
	for s.in.Len() < n {
		p, err := s.data()
		if err != nil {
			return nil, s.fail(err)
		}
		s.in.Write(p)
	}
	b := make([]byte, n)
	_, err := io.ReadFull(&s.in, b)
	return b, err
}
func (s *session) GetByte() (byte, error) {
	b, e := s.GetBytes(1)
	if e != nil {
		return 0, e
	}
	return b[0], nil
}
func (s *session) GetInt(size int, compressed, bigEndian bool) (int, error) {
	if compressed {
		b, e := s.GetByte()
		if e != nil {
			return 0, e
		}
		size = int(b)
		if size&0x80 != 0 {
			return 0, s.fail(fmt.Errorf("oracle: negative wire integer"))
		}
		bigEndian = true
	}
	if size < 0 || size > 8 {
		return 0, s.fail(fmt.Errorf("oracle: invalid integer width %d", size))
	}
	b, e := s.GetBytes(size)
	if e != nil {
		return 0, e
	}
	var v uint64
	if bigEndian {
		for _, x := range b {
			v = v<<8 | uint64(x)
		}
	} else {
		for i := len(b) - 1; i >= 0; i-- {
			v = v<<8 | uint64(b[i])
		}
	}
	if v > uint64(^uint(0)>>1) {
		return 0, s.fail(fmt.Errorf("oracle: integer overflow"))
	}
	return int(v), nil
}
func (s *session) count(max int) (int, error) {
	n, e := s.GetInt(4, true, true)
	if e != nil {
		return 0, e
	}
	if n > max {
		return 0, s.fail(fmt.Errorf("oracle: count %d exceeds %d", n, max))
	}
	return n, nil
}
func (s *session) GetNullTermString() (string, error) {
	var b []byte
	for len(b) < 1024 {
		x, e := s.GetByte()
		if e != nil {
			return "", e
		}
		if x == 0 {
			return string(b), nil
		}
		b = append(b, x)
	}
	return "", s.fail(fmt.Errorf("oracle: unterminated string"))
}
func (s *session) GetClr() ([]byte, error) {
	n, e := s.GetByte()
	if e != nil {
		return nil, e
	}
	if n == 0 || n == 0xff || n == 0xfd {
		return nil, nil
	}
	if n != 0xfe {
		return s.GetBytes(int(n))
	}
	var out []byte
	for {
		var size int
		if s.UseBigClrChunks {
			size, e = s.GetInt(4, true, true)
		} else {
			n, e = s.GetByte()
			size = int(n)
		}
		if e != nil {
			return nil, e
		}
		if size == 0 {
			return out, nil
		}
		if size > maxField-len(out) {
			return nil, s.fail(fmt.Errorf("oracle: CLR exceeds limit"))
		}
		part, e := s.GetBytes(size)
		if e != nil {
			return nil, e
		}
		out = append(out, part...)
	}
}
func (s *session) GetDlc() ([]byte, error) {
	n, e := s.count(maxField)
	if e != nil || n == 0 {
		return nil, e
	}
	b, e := s.GetClr()
	if e != nil {
		return nil, e
	}
	if len(b) != n {
		return nil, s.fail(fmt.Errorf("oracle: DLC length mismatch"))
	}
	return b, nil
}
func (s *session) GetKeyVal() ([]byte, []byte, int, error) {
	k, e := s.GetDlc()
	if e != nil {
		return nil, nil, 0, e
	}
	v, e := s.GetDlc()
	if e != nil {
		return nil, nil, 0, e
	}
	f, e := s.GetInt(4, true, true)
	return k, v, f, e
}
func (s *session) PutBytes(b ...byte) { s.out.Write(b) }
func (s *session) PutInt(v interface{}, size int, bigEndian, compressed bool) {
	var n uint64
	switch v := v.(type) {
	case int:
		n = uint64(v)
	case uint8:
		n = uint64(v)
	case uint16:
		n = uint64(v)
	case uint32:
		n = uint64(v)
	case uint64:
		n = v
	default:
		s.fail(fmt.Errorf("oracle: invalid local integer type %T", v))
		return
	}
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], n)
	if compressed {
		i := 0
		for i < 8 && b[i] == 0 {
			i++
		}
		s.out.WriteByte(byte(8 - i))
		s.out.Write(b[i:])
		return
	}
	if size < 1 || size > 8 {
		s.fail(fmt.Errorf("oracle: invalid local integer width"))
		return
	}
	if bigEndian {
		s.out.Write(b[8-size:])
	} else {
		for i := 7; i >= 8-size; i-- {
			s.out.WriteByte(b[i])
		}
	}
}
func (s *session) PutUint(v interface{}, size int, bigEndian, compressed bool) {
	s.PutInt(v, size, bigEndian, compressed)
}
func (s *session) PutClr(b []byte) {
	if len(b) <= 252 {
		s.out.WriteByte(byte(len(b)))
		s.out.Write(b)
		return
	}
	s.out.WriteByte(0xfe)
	for len(b) > 0 {
		n := min(s.ClrChunkSize, len(b))
		if s.UseBigClrChunks {
			s.PutInt(n, 4, true, true)
		} else {
			s.out.WriteByte(byte(n))
		}
		s.out.Write(b[:n])
		b = b[n:]
	}
	s.out.WriteByte(0)
}
func (s *session) PutString(v string) { s.PutClr([]byte(v)) }
func (s *session) PutKeyValString(k, v string, flag byte) {
	for _, x := range []string{k, v} {
		s.PutInt(len(x), 4, true, true)
		if len(x) > 0 {
			s.PutString(x)
		}
	}
	s.PutInt(flag, 4, true, true)
}
func (s *session) PutTTCFunc(code, fn byte) {
	s.sequence++
	if s.sequence == 0 {
		s.sequence = 1
	}
	s.PutBytes(code, fn, s.sequence)
	if s.TTCVersion >= 18 {
		s.PutBytes(0)
	}
}
func (s *session) ResetBuffer() { s.out.Reset() }
func (s *session) Write() error {
	if s.err != nil {
		return s.err
	}
	if s.in.Len() != 0 {
		return s.fail(fmt.Errorf("oracle: unread response bytes before next request"))
	}
	return s.writeData(bytes.Clone(s.out.Bytes()))
}
