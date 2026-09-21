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
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

// RPMSENSE comparison and qualifier bits from rpm/lib/rpmds.h (rpm 4.14).
const (
	rpmSenseLess    = 1 << 1
	rpmSenseGreater = 1 << 2
	rpmSenseEqual   = 1 << 3
)

type Dependency struct {
	Name, Version string
	Flags         uint32
}

func (d Dependency) Constraint() string {
	var op string
	if d.Flags&rpmSenseLess != 0 {
		op += "<"
	}
	if d.Flags&rpmSenseGreater != 0 {
		op += ">"
	}
	if d.Flags&rpmSenseEqual != 0 {
		op += "="
	}
	if op == "" {
		return d.Version
	}
	if d.Version == "" {
		return op
	}
	return op + " " + d.Version
}

type PackageInfo struct {
	Name, Version, Release, Arch, License, SigMD5 string
	Epoch                                         uint32
	Provides, Requires                            []string
	ProvideDeps, RequireDeps                      []Dependency
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
			return scanerr.New(scanerr.ResourceLimit, "RPM records")
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
		return nil, scanerr.Wrap(scanerr.Cancelled, err)
	}
	if off < 0 || n < 0 || int64(n) > s.size || off > s.size-int64(n) {
		return nil, scanerr.New(scanerr.MalformedInput, "RPM read outside snapshot")
	}
	s.visits++
	if s.visits > s.limits.MaxPageVisits || int64(n) > s.limits.MaxReadBytes-s.read {
		return nil, scanerr.New(scanerr.ResourceLimit, "RPM cumulative reads")
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
		return nil, scanerr.New(scanerr.MalformedInput, "short RPM header")
	}
	ni, dl := uint64(be.Uint32(b)), uint64(be.Uint32(b[4:]))
	start := 8 + 16*ni
	if ni == 0 || ni > 65536 || start+dl > uint64(len(b)) {
		return nil, scanerr.New(scanerr.MalformedInput, "RPM header dimensions")
	}
	data := b[start : start+dl]
	p := &PackageInfo{}
	valuesRead := 0
	seen := map[uint32]bool{}
	strTag := map[uint32][]string{}
	intTag := map[uint32][]uint32{}
	storeString := func(tag uint32) bool {
		return tag == 1000 || tag == 1001 || tag == 1002 || tag == 1014 || tag == 1022 || tag == 1047 || tag == 1049 || tag == 1050 || tag == 1113
	}
	for i := uint64(0); i < ni; i++ {
		if err := ctx.Err(); err != nil {
			return nil, scanerr.Wrap(scanerr.Cancelled, err)
		}
		e := b[8+16*i : 8+16*(i+1)]
		tag, typ, off, count := be.Uint32(e), be.Uint32(e[4:]), uint64(be.Uint32(e[8:])), uint64(be.Uint32(e[12:]))
		if typ > 9 || off > dl || count > 1000000 {
			return nil, scanerr.New(scanerr.MalformedInput, "RPM tag dimensions")
		}
		var values []string
		var raw []byte
		switch typ {
		case 6, 8, 9:
			if typ == 6 && count != 1 {
				return nil, scanerr.New(scanerr.MalformedInput, "RPM string count")
			}
			rest := data[off:]
			used := 0
			for j := uint64(0); j < count; j++ {
				if j&255 == 0 {
					if err := ctx.Err(); err != nil {
						return nil, scanerr.Wrap(scanerr.Cancelled, err)
					}
				}
				if valuesRead >= limits.MaxResolveSteps {
					return nil, scanerr.New(scanerr.ResourceLimit, "RPM header traversal")
				}
				valuesRead++
				k := bytes.IndexByte(rest, 0)
				if k < 0 {
					return nil, scanerr.New(scanerr.MalformedInput, "unterminated RPM string")
				}
				if k > limits.MaxFieldBytes {
					return nil, scanerr.New(scanerr.ResourceLimit, "RPM field")
				}
				if storeString(tag) {
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
				return nil, scanerr.New(scanerr.MalformedInput, "RPM tag out of bounds")
			}
			raw = data[off : off+n]
		}
		seen[tag] = true
		switch tag {
		case 1000, 1001, 1002, 1014, 1022:
			if typ != 6 || len(values) != 1 {
				return nil, scanerr.New(scanerr.MalformedInput, "RPM scalar tag %d", tag)
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
				return nil, scanerr.New(scanerr.MalformedInput, "RPM epoch")
			}
			p.Epoch = be.Uint32(raw)
		case 261:
			if typ != 7 || count != 16 {
				return nil, scanerr.New(scanerr.MalformedInput, "RPM MD5")
			}
			p.SigMD5 = hex.EncodeToString(raw)
		case 1047, 1049, 1050, 1113:
			if typ != 8 {
				return nil, scanerr.New(scanerr.MalformedInput, "RPM dependency type")
			}
			strTag[tag] = values
			if tag == 1047 {
				p.Provides = values
			}
			if tag == 1049 {
				p.Requires = values
			}
		case 1048, 1112:
			if typ != 4 {
				return nil, scanerr.New(scanerr.MalformedInput, "RPM dependency flags")
			}
			ints := make([]uint32, count)
			for i := range ints {
				ints[i] = be.Uint32(raw[4*i:])
			}
			intTag[tag] = ints
		}
	}
	var err error
	p.ProvideDeps, err = alignRPMDeps(strTag[1047], strTag[1113], intTag[1112], seen[1047], seen[1113], seen[1112])
	if err != nil {
		return nil, err
	}
	p.RequireDeps, err = alignRPMDeps(strTag[1049], strTag[1050], intTag[1048], seen[1049], seen[1050], seen[1048])
	if err != nil {
		return nil, err
	}
	if p.Name == "" || p.Version == "" {
		return nil, scanerr.New(scanerr.MalformedInput, "missing RPM identity")
	}
	return p, nil
}

func alignRPMDeps(names, versions []string, flags []uint32, haveNames, haveVersions, haveFlags bool) ([]Dependency, error) {
	if !haveNames {
		if haveVersions || haveFlags {
			return nil, scanerr.New(scanerr.MalformedInput, "RPM dependency arrays without names")
		}
		return nil, nil
	}
	if haveVersions && len(versions) != len(names) {
		return nil, scanerr.New(scanerr.MalformedInput, "RPM dependency name/version length")
	}
	if haveFlags && len(flags) != len(names) {
		return nil, scanerr.New(scanerr.MalformedInput, "RPM dependency name/flag length")
	}
	if !haveVersions {
		versions = make([]string, len(names))
	}
	if !haveFlags {
		flags = make([]uint32, len(names))
	}
	out := make([]Dependency, len(names))
	for i, name := range names {
		if (flags[i]&(rpmSenseLess|rpmSenseGreater|rpmSenseEqual) != 0) && versions[i] == "" {
			return nil, scanerr.New(scanerr.MalformedInput, "RPM dependency comparison without version")
		}
		out[i] = Dependency{Name: name, Version: versions[i], Flags: flags[i]}
	}
	return out, nil
}
