package pcaputil

import (
	"fmt"
	"google.golang.org/protobuf/encoding/protowire"
	"math"
	"unicode/utf8"
)

// Sparkplug 3.0 DataSet: explicitly described, rectangular Basic scalar rows.
// The original protobuf message body remains an owned dataset_value byte slice.
// Unknown fields are preserved there; no unknown extension is guessed as null.
func sparkDataSet(w []byte, maxDepth, maxFields int) (map[string]any, string) {
	columns := []string{}
	types := []uint64{}
	rowWires := [][]byte{}
	var declared uint64
	present := false
	count := 0
	err := sparkWalk(w, 3, maxDepth, maxFields, &count, func(n protowire.Number, t protowire.Type, v []byte) error {
		_, _, tag := protowire.ConsumeTag(v)
		v = v[tag:]
		switch {
		case n == 1 && t == protowire.VarintType:
			declared, _ = protowire.ConsumeVarint(v)
			present = true
		case n == 2 && t == protowire.BytesType:
			b, _ := protowire.ConsumeBytes(v)
			if !utf8.Valid(b) {
				return fmt.Errorf("Sparkplug invalid UTF-8 DataSet column")
			}
			columns = append(columns, string(b))
		case n == 3 && t == protowire.VarintType:
			u, _ := protowire.ConsumeVarint(v)
			types = append(types, uint64(uint32(u)))
		case n == 3 && t == protowire.BytesType:
			b, _ := protowire.ConsumeBytes(v)
			for len(b) > 0 {
				u, k := protowire.ConsumeVarint(b)
				if k < 0 {
					return fmt.Errorf("Sparkplug truncated packed DataSet types")
				}
				types = append(types, uint64(uint32(u)))
				b = b[k:]
			}
		case n == 4 && t == protowire.BytesType:
			b, _ := protowire.ConsumeBytes(v)
			rowWires = append(rowWires, b)
		}
		return nil
	})
	if err != nil {
		return nil, err.Error()
	}
	if !present || declared == 0 || len(columns) != len(types) || declared != uint64(len(types)) {
		return nil, "DataSet requires complete equal column names, types and declared count"
	}
	for _, typ := range types {
		if typ < 1 || typ > 14 {
			return nil, "DataSet requires a supported Basic scalar column type"
		}
	}
	rows := make([]any, 0, len(rowWires))
	for _, rw := range rowWires {
		row := []any{}
		err = sparkWalk(rw, 4, maxDepth, maxFields, &count, func(n protowire.Number, t protowire.Type, v []byte) error {
			if n != 1 || t != protowire.BytesType {
				return nil
			}
			if len(row) >= len(types) {
				return fmt.Errorf("DataSet row differs from declared column count")
			}
			_, _, tag := protowire.ConsumeTag(v)
			cw, _ := protowire.ConsumeBytes(v[tag:])
			cell, why := sparkDataSetCell(cw, types[len(row)], maxDepth, maxFields, &count)
			if why != "" {
				return fmt.Errorf("%s", why)
			}
			row = append(row, cell)
			return nil
		})
		if err != nil {
			return nil, err.Error()
		}
		if len(row) != len(types) {
			return nil, "DataSet row differs from declared column count"
		}
		rows = append(rows, row)
	}
	return map[string]any{"num_of_columns": declared, "columns": columns, "types": types, "rows": rows}, ""
}

func sparkDataSetCell(w []byte, typ uint64, maxDepth, maxFields int, count *int) (any, string) {
	m := map[string]any{}
	keys := map[protowire.Number]string{1: "int_value", 2: "long_value", 3: "float_value", 4: "double_value", 5: "boolean_value", 6: "string_value", 7: "extension_value"}
	err := sparkWalk(w, 5, maxDepth, maxFields, count, func(n protowire.Number, t protowire.Type, v []byte) error {
		key, known := keys[n]
		if !known {
			return nil
		}
		want := protowire.VarintType
		switch n {
		case 3:
			want = protowire.Fixed32Type
		case 4:
			want = protowire.Fixed64Type
		case 6, 7:
			want = protowire.BytesType
		}
		if t != want {
			return nil
		}
		for _, k := range keys {
			delete(m, k)
		}
		_, _, tag := protowire.ConsumeTag(v)
		v = v[tag:]
		switch n {
		case 1, 2, 5:
			u, _ := protowire.ConsumeVarint(v)
			if n == 1 {
				m[key] = uint64(uint32(u))
			} else if n == 5 {
				m[key] = u != 0
			} else {
				m[key] = u
			}
		case 3:
			u, _ := protowire.ConsumeFixed32(v)
			m[key] = math.Float32frombits(u)
		case 4:
			u, _ := protowire.ConsumeFixed64(v)
			m[key] = math.Float64frombits(u)
		case 6:
			b, _ := protowire.ConsumeBytes(v)
			if !utf8.Valid(b) {
				return fmt.Errorf("Sparkplug invalid UTF-8 DataSet cell")
			}
			m[key] = string(b)
		case 7:
			m[key] = true
		}
		return nil
	})
	if err != nil {
		return nil, err.Error()
	}
	if m["extension_value"] != nil {
		return nil, "DataSet extension cell needs an independent profile"
	}
	value, why := sparkValue(m, typ, maxDepth, maxFields)
	if why != "" {
		return nil, "DataSet cell: " + why
	}
	return value, ""
}

// A single allocation-free budget pass traverses every known nested envelope,
// including overwritten oneofs. Packed type elements count individually. Cost
// is a conservative high-water allowance for the new public graph and snapshots;
// declarations never drive allocations. Unknown length-delimited fields stay opaque.
func sparkPreflightWithCount(w []byte, maxDepth, maxFields int) (int64, int, error) {
	count := 0
	var cost int64
	var walk func([]byte, int, int) error
	walk = func(w []byte, depth, kind int) error {
		return sparkWalk(w, depth, maxDepth, maxFields, &count, func(n protowire.Number, t protowire.Type, v []byte) error {
			if kind >= 2 {
				cost += 256
			}
			_, _, tag := protowire.ConsumeTag(v)
			v = v[tag:]
			if kind == 2 && n == 3 && t == protowire.BytesType {
				b, _ := protowire.ConsumeBytes(v)
				for len(b) > 0 {
					_, k := protowire.ConsumeVarint(b)
					if k < 0 {
						return fmt.Errorf("Sparkplug truncated packed DataSet types")
					}
					count++
					cost += 256
					if count > maxFields {
						return protocolError(ErrResourceExceeded, "Sparkplug protobuf fields")
					}
					b = b[k:]
				}
				return nil
			}
			if t != protowire.BytesType {
				return nil
			}
			next := -1
			switch {
			case kind == 0 && n == 2:
				next = 1
			case kind == 1 && n == 17:
				next = 2
			case kind == 1 && n == 18:
				next = 5
			case kind == 5 && n == 2:
				next = 1
			case kind == 5 && n == 3:
				next = 6
			case kind == 2 && n == 4:
				next = 3
			case kind == 3 && n == 1:
				next = 4
			}
			if next < 0 {
				return nil
			}
			b, _ := protowire.ConsumeBytes(v)
			if next == 2 || next == 5 {
				cost += 512 + 8*int64(len(b))
			}
			return walk(b, depth+1, next)
		})
	}
	err := walk(w, 1, 0)
	return cost, count, err
}

func sparkPreflight(w []byte, maxDepth, maxFields int) (int64, error) {
	cost, _, err := sparkPreflightWithCount(w, maxDepth, maxFields)
	return cost, err
}
