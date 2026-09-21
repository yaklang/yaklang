package rpm

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

// This is a reader for the frozen RPM Packages(hnum,blob) layout, based on
// sqlite.org/fileformat2.html. It does not parse/execute SQL, load extensions,
// replay journals, or interpret arbitrary user tables.
type sqliteSnapshot struct {
	s                *snapshot
	pageSize, usable int
	pages            uint32
	seen             map[uint32]bool
	rows             int
}

func (s *snapshot) sqlite(h []byte, emit func([]byte) error) error {
	size := int(be.Uint16(h[16:]))
	if size == 1 {
		size = 65536
	}
	if size < 512 || size > 65536 || size&(size-1) != 0 || s.size%int64(size) != 0 {
		return fmt.Errorf("malformed_input: SQLite page size")
	}
	if h[18] < 1 || h[18] > 2 || h[19] < 1 || h[19] > 2 || h[20] != 0 || !bytes.Equal(h[21:24], []byte{64, 32, 32}) || be.Uint32(h[56:]) != 1 {
		return fmt.Errorf("unsupported_syntax: SQLite header layout")
	}
	pages := uint32(s.size / int64(size))
	decl := be.Uint32(h[28:])
	if decl != 0 && be.Uint32(h[24:]) == be.Uint32(h[92:]) {
		if decl > pages {
			return fmt.Errorf("malformed_input: SQLite truncated snapshot")
		}
		pages = decl
	}
	db := &sqliteSnapshot{s: s, pageSize: size, usable: size, pages: pages, seen: map[uint32]bool{}}
	var root uint32
	err := db.table(1, 0, func(raw []byte) error {
		fields, err := sqliteRecord(raw)
		if err != nil {
			return err
		}
		if len(fields) != 5 {
			return fmt.Errorf("malformed_input: SQLite schema record")
		}
		if string(fields[1].raw) != "Packages" {
			return nil
		}
		if root != 0 || string(fields[0].raw) != "table" || string(fields[2].raw) != "Packages" {
			return fmt.Errorf("unsupported_syntax: SQLite Packages schema")
		}
		// These are the two schema strings emitted by the retained RPM backend.
		sql := strings.ToLower(strings.Join(strings.Fields(string(fields[4].raw)), ""))
		sql = strings.NewReplacer("'", "", "\"", "", "`", "", "[", "", "]", "").Replace(sql)
		if sql != "createtablepackages(hnumintegerprimarykeyautoincrement,blobblobnotnull)" && sql != "createtablepackages(hnumintegerprimarykey,blobblobnotnull)" {
			return fmt.Errorf("unsupported_syntax: SQLite Packages columns")
		}
		n, err := fields[3].integer()
		if err != nil || n == 0 || n > uint64(pages) {
			return fmt.Errorf("malformed_input: SQLite table root")
		}
		root = uint32(n)
		return nil
	})
	if err != nil {
		return err
	}
	if root == 0 {
		return fmt.Errorf("evidence_insufficient: SQLite Packages table missing")
	}
	return db.table(root, 0, func(raw []byte) error {
		fields, err := sqliteRecord(raw)
		if err != nil {
			return err
		}
		if len(fields) != 2 || fields[0].typ != 0 || fields[1].typ < 12 || fields[1].typ%2 != 0 {
			return fmt.Errorf("malformed_input: SQLite RPM record layout")
		}
		return emit(fields[1].raw)
	})
}
func (db *sqliteSnapshot) page(number uint32) ([]byte, error) {
	if number == 0 || number > db.pages || db.seen[number] {
		return nil, fmt.Errorf("malformed_input: SQLite page cycle, overlap, or bounds")
	}
	db.seen[number] = true
	return db.s.at(int64(number-1)*int64(db.pageSize), db.pageSize)
}
func (db *sqliteSnapshot) table(number uint32, depth int, emit func([]byte) error) error {
	if depth > 64 {
		return scanerr.New(scanerr.ResourceLimit, "SQLite btree depth")
	}
	b, err := db.page(number)
	if err != nil {
		return err
	}
	base := 0
	if number == 1 {
		base = 100
	}
	kind := b[base]
	hs := 8
	if kind == 5 {
		hs = 12
	} else if kind != 13 {
		return fmt.Errorf("unsupported_syntax: SQLite non-table btree")
	}
	count := int(be.Uint16(b[base+3:]))
	ptr := base + hs
	if ptr+count*2 > db.usable {
		return fmt.Errorf("malformed_input: SQLite cell pointers")
	}
	for i := 0; i < count; i++ {
		off := int(be.Uint16(b[ptr+2*i:]))
		if off < ptr+count*2 || off >= db.usable {
			return fmt.Errorf("malformed_input: SQLite cell offset")
		}
		cell := b[off:db.usable]
		if kind == 5 {
			if len(cell) < 5 {
				return fmt.Errorf("malformed_input: SQLite interior cell")
			}
			if _, _, err := sqliteVarint(cell[4:]); err != nil {
				return err
			}
			if err = db.table(be.Uint32(cell), depth+1, emit); err != nil {
				return err
			}
			continue
		}
		plen, n, err := sqliteVarint(cell)
		if err != nil {
			return err
		}
		if plen > uint64(db.s.limits.MaxRecordBytes) {
			return scanerr.New(scanerr.ResourceLimit, "SQLite payload")
		}
		_, rn, err := sqliteVarint(cell[n:])
		if err != nil {
			return err
		}
		cell = cell[n+rn:]
		local := int(plen)
		if local > db.usable-35 {
			minimum := (db.usable-12)*32/255 - 23
			local = minimum + (int(plen)-minimum)%(db.usable-4)
			if local > db.usable-35 {
				local = minimum
			}
		}
		if local > len(cell) {
			return fmt.Errorf("malformed_input: SQLite local payload")
		}
		raw := make([]byte, 0, int(plen))
		raw = append(raw, cell[:local]...)
		if local < int(plen) {
			if local+4 > len(cell) {
				return fmt.Errorf("malformed_input: SQLite overflow pointer")
			}
			next := be.Uint32(cell[local:])
			for len(raw) < int(plen) {
				page, err := db.page(next)
				if err != nil {
					return err
				}
				next = be.Uint32(page)
				n := min(int(plen)-len(raw), db.usable-4)
				raw = append(raw, page[4:4+n]...)
			}
			if next != 0 {
				return fmt.Errorf("malformed_input: SQLite surplus overflow")
			}
		}
		db.rows++
		if db.rows > db.s.limits.MaxRecords+10000 {
			return scanerr.New(scanerr.ResourceLimit, "SQLite rows")
		}
		if err = emit(raw); err != nil {
			return err
		}
	}
	if kind == 5 {
		return db.table(be.Uint32(b[base+8:]), depth+1, emit)
	}
	return nil
}
func sqliteVarint(b []byte) (uint64, int, error) {
	var v uint64
	for i := 0; i < 9; i++ {
		if i >= len(b) {
			return 0, 0, fmt.Errorf("malformed_input: SQLite truncated varint")
		}
		if i == 8 {
			return v<<8 | uint64(b[i]), 9, nil
		}
		v = v<<7 | uint64(b[i]&127)
		if b[i] < 128 {
			return v, i + 1, nil
		}
	}
	panic("unreachable")
}

type sqliteField struct {
	typ uint64
	raw []byte
}

func (f sqliteField) integer() (uint64, error) {
	if f.typ == 8 {
		return 0, nil
	}
	if f.typ == 9 {
		return 1, nil
	}
	if f.typ < 1 || f.typ > 6 || len(f.raw) == 0 || f.raw[0]&128 != 0 {
		return 0, fmt.Errorf("malformed_input: SQLite unsigned integer")
	}
	var n uint64
	for _, b := range f.raw {
		n = n<<8 | uint64(b)
	}
	return n, nil
}
func sqliteRecord(b []byte) ([]sqliteField, error) {
	header, n, err := sqliteVarint(b)
	if err != nil {
		return nil, err
	}
	if header < uint64(n) || header > uint64(len(b)) {
		return nil, fmt.Errorf("malformed_input: SQLite record header")
	}
	data := b[header:]
	var result []sqliteField
	for n < int(header) {
		typ, k, err := sqliteVarint(b[n:header])
		if err != nil {
			return nil, err
		}
		n += k
		var size uint64
		switch {
		case typ <= 9:
			size = []uint64{0, 1, 2, 3, 4, 6, 8, 8, 0, 0}[typ]
		case typ < 12:
			return nil, fmt.Errorf("malformed_input: reserved SQLite serial type")
		default:
			size = (typ - 12) / 2
		}
		if size > uint64(len(data)) {
			return nil, fmt.Errorf("malformed_input: SQLite field length")
		}
		result = append(result, sqliteField{typ: typ, raw: data[:size]})
		data = data[size:]
		if len(result) > 64 {
			return nil, fmt.Errorf("unsupported_syntax: SQLite column count")
		}
	}
	if len(data) != 0 {
		return nil, fmt.Errorf("malformed_input: SQLite trailing record bytes")
	}
	return result, nil
}
