// Package rpm extracts package headers from immutable RPM database snapshots.
// Format layouts derive from go-rpmdb v0.1.0 (MIT, see LICENSE and source map).
// It has no SQL engine, database driver, filename API, locking, or goroutines.
package rpm

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/sca/core/budget"
)

type PackageInfo struct {
	Name, Version, Release, Arch, License, SigMD5 string
	Epoch                                         uint32
	Provides, Requires                            []string
}
type Limits struct {
	MaxReadBytes                              int64
	MaxRecords, MaxRecordBytes, MaxPageVisits int
}
type snapshot struct {
	ctx             context.Context
	r               io.ReaderAt
	size, read      int64
	limits          Limits
	visits, records int
}

func Parse(ctx context.Context, r io.ReaderAt, size int64, lim Limits) ([]*PackageInfo, error) {
	if lim.MaxReadBytes < 0 || lim.MaxRecords < 0 || lim.MaxRecordBytes < 0 || lim.MaxPageVisits < 0 {
		return nil, fmt.Errorf("invalid RPM limits")
	}
	if lim.MaxReadBytes == 0 {
		lim.MaxReadBytes = 512 << 20
	}
	if lim.MaxRecords == 0 {
		lim.MaxRecords = 200000
	}
	if lim.MaxRecordBytes == 0 {
		lim.MaxRecordBytes = 16 << 20
	}
	if lim.MaxPageVisits == 0 {
		lim.MaxPageVisits = 1000000
	}
	scanLimits := budget.From(ctx).Limits
	lim.MaxRecords = min(lim.MaxRecords, scanLimits.MaxComponents)
	lim.MaxRecordBytes = min(lim.MaxRecordBytes, int(scanLimits.MaxFileBytes))
	lim.MaxReadBytes = min(lim.MaxReadBytes, scanLimits.MaxTotalReadBytes)
	lim.MaxPageVisits = min(lim.MaxPageVisits, scanLimits.MaxResolveSteps)
	s := &snapshot{ctx: ctx, r: r, size: size, limits: lim}
	page := lim.MaxRecordBytes
	if page <= 0 || page > 16<<20 {
		page = 16 << 20
	}
	if err := budget.From(ctx).Result(budget.SizeOfBytes(page)); err != nil {
		return nil, err
	}
	head, err := s.at(0, 100)
	if err != nil {
		return nil, err
	}
	var result []*PackageInfo
	emit := func(raw []byte) error {
		s.records++
		if s.records > lim.MaxRecords {
			return fmt.Errorf("resource_limit: RPM records")
		}
		p, err := headerWithContext(ctx, raw, scanLimits)
		if err != nil {
			return err
		}
		if err := budget.From(ctx).Result(budget.SizeOfPackage(p.Name, p.Version, p.License)); err != nil {
			return err
		}
		result = append(result, p)
		return nil
	}
	switch {
	case bytes.HasPrefix(head, []byte("SQLite format 3\x00")):
		err = s.sqlite(head, emit)
	case string(head[:4]) == "RpmP":
		err = s.ndb(head, emit)
	default:
		err = s.bdb(head, emit)
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *snapshot) at(off int64, n int) ([]byte, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	if off < 0 || n < 0 || int64(n) > s.size || off > s.size-int64(n) {
		return nil, fmt.Errorf("malformed_input: RPM read outside snapshot")
	}
	s.visits++
	if s.visits > s.limits.MaxPageVisits || int64(n) > s.limits.MaxReadBytes-s.read {
		return nil, fmt.Errorf("resource_limit: RPM cumulative reads")
	}
	s.read += int64(n)
	b := make([]byte, n)
	if _, err := s.r.ReadAt(b, off); err != nil {
		return nil, fmt.Errorf("RPM read: %w", err)
	}
	return b, nil
}

var be = binary.BigEndian
var le = binary.LittleEndian

// header reads only the tags used by SCA. It validates every entry's bounds,
// type, and count first; irrelevant tags cannot hide malformed lengths.
// Dribble entries override earlier values of the same tag, as RPM specifies.
func header(b []byte) (*PackageInfo, error) {
	return headerWithContext(context.Background(), b, budget.From(context.Background()).Limits)
}

func headerWithContext(ctx context.Context, b []byte, limits budget.Limits) (*PackageInfo, error) {
	if len(b) < 8 {
		return nil, fmt.Errorf("malformed_input: short RPM header")
	}
	ni, dl := uint64(be.Uint32(b)), uint64(be.Uint32(b[4:]))
	start := 8 + 16*ni
	if ni == 0 || ni > 65536 || start+dl > uint64(len(b)) {
		return nil, fmt.Errorf("malformed_input: RPM header dimensions")
	}
	data := b[start : start+dl]
	p := &PackageInfo{}
	valuesRead := 0
	for i := uint64(0); i < ni; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		e := b[8+16*i : 8+16*(i+1)]
		tag, typ, off, count := be.Uint32(e), be.Uint32(e[4:]), uint64(be.Uint32(e[8:])), uint64(be.Uint32(e[12:]))
		if typ > 9 || off > dl || count > 1000000 {
			return nil, fmt.Errorf("malformed_input: RPM tag dimensions")
		}
		var values []string
		var raw []byte
		switch typ {
		case 6, 8, 9:
			if typ == 6 && count != 1 {
				return nil, fmt.Errorf("malformed_input: RPM string count")
			}
			rest := data[off:]
			used := 0
			for j := uint64(0); j < count; j++ {
				if j&255 == 0 {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
				}
				if valuesRead >= limits.MaxResolveSteps {
					return nil, fmt.Errorf("resource_limit: RPM header traversal")
				}
				valuesRead++
				k := bytes.IndexByte(rest, 0)
				if k < 0 {
					return nil, fmt.Errorf("malformed_input: unterminated RPM string")
				}
				if k > limits.MaxFieldBytes {
					return nil, fmt.Errorf("resource_limit: RPM field")
				}
				if tag == 1000 || tag == 1001 || tag == 1002 || tag == 1014 || tag == 1022 || tag == 1047 || tag == 1049 {
					if err := budget.From(ctx).Result(budget.SizeString + int64(k)); err != nil {
						return nil, err
					}
					values = append(values, string(rest[:k]))
				}
				rest = rest[k+1:]
				used += k + 1
			}
			raw = data[off : off+uint64(used)]
		default:
			sizes := [10]uint64{0, 1, 1, 2, 4, 8, 0, 1, 0, 0}
			n := count * sizes[typ]
			if n > dl-off {
				return nil, fmt.Errorf("malformed_input: RPM tag out of bounds")
			}
			raw = data[off : off+n]
		}
		switch tag {
		case 1000, 1001, 1002, 1014, 1022:
			if typ != 6 || len(values) != 1 {
				return nil, fmt.Errorf("malformed_input: RPM scalar tag %d", tag)
			}
			switch tag {
			case 1000:
				p.Name = values[0]
			case 1001:
				p.Version = values[0]
			case 1002:
				p.Release = values[0]
			case 1014:
				p.License = values[0]
			case 1022:
				p.Arch = values[0]
			}
		case 1003:
			if typ != 4 || count != 1 {
				return nil, fmt.Errorf("malformed_input: RPM epoch")
			}
			p.Epoch = be.Uint32(raw)
		case 261:
			if typ != 7 || count != 16 {
				return nil, fmt.Errorf("malformed_input: RPM MD5")
			}
			p.SigMD5 = hex.EncodeToString(raw)
		case 1047, 1049:
			if typ != 8 {
				return nil, fmt.Errorf("malformed_input: RPM dependency type")
			}
			if tag == 1047 {
				p.Provides = values
			} else {
				p.Requires = values
			}
		}
	}
	if p.Name == "" || p.Version == "" {
		return nil, fmt.Errorf("malformed_input: missing RPM identity")
	}
	return p, nil
}
