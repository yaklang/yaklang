package pcaputil

// Selected W342-E1-18 read-only layouts. Area codes describe a wire layout,
// never an inferred PLC model, installed memory map or successful execution.
func finsMemoryType(area byte, multiple bool) (string, int) {
	switch area {
	case 0x02, 0x30, 0x31, 0x32, 0x33:
		return "bit", 1
	case 0x82, 0xb0, 0xb1, 0xb2, 0xb3:
		return "word", 2
	case 0x70, 0x71, 0x72:
		if multiple {
			return "forced-bit", 1
		}
	case 0xf0, 0xf1, 0xf2:
		if multiple {
			return "forced-word", 4
		}
	}
	return "", 0
}
func finsMemoryDescriptor(w []byte, multiple bool) (map[string]any, string, int, error) {
	if len(w) != 4 || w[3] > 15 {
		return nil, "", 0, finsErr(ErrMalformedMessage, "memory descriptor/bit address")
	}
	kind, width := finsMemoryType(w[0], multiple)
	if width == 0 {
		return nil, "", 0, finsErr(ErrUnsupportedFeature, "selected bit/word memory layout only")
	}
	if width > 1 && w[3] != 0 {
		return nil, "", 0, finsErr(ErrMalformedMessage, "word address bit must be zero")
	}
	return map[string]any{"memory_area": uint64(w[0]), "word_address": finsBE(w[1:3]), "bit_address": uint64(w[3]), "element_type": kind}, kind, width, nil
}
func finsMemoryRequest(command uint16, w []byte, f map[string]any, b ParserBudget) error {
	if command == 0x0101 {
		if len(w) != 18 {
			return finsErr(ErrMalformedMessage, "memory read request exact length")
		}
		d, kind, _, err := finsMemoryDescriptor(w[12:16], false)
		if err != nil {
			return err
		}
		f["memory_area"], f["word_address"], f["bit_address"] = d["memory_area"], d["word_address"], d["bit_address"]
		if kind == "word" {
			f["word_count"] = finsBE(w[16:18])
		} else {
			f["item_count"], f["element_type"] = finsBE(w[16:18]), kind
		}
		return nil
	}
	if (len(w)-12)%4 != 0 {
		return finsErr(ErrMalformedMessage, "multiple read descriptor alignment")
	}
	n := (len(w) - 12) / 4
	if n > 167 {
		return finsErr(ErrUnsupportedFeature, "selected Ethernet multiple read167 items")
	}
	if n > (b.MaxCollectionElements-32)/8 {
		return finsErr(ErrResourceExceeded, "multiple read descriptor projection")
	}
	items := make([]any, 0, n)
	for pos := 12; pos < len(w); pos += 4 {
		d, _, _, err := finsMemoryDescriptor(w[pos:pos+4], true)
		if err != nil {
			return err
		}
		items = append(items, d)
	}
	f["item_count"], f["memory_items"] = uint64(n), items
	return nil
}
func finsMemoryResponse(command uint16, data []byte, p *finsPending, linked bool, f map[string]any, b ParserBudget) error {
	if !linked {
		return finsErr(ErrContextRequired, "read response lacks exact reversed request/descriptors context")
	}
	if command == 0x0101 {
		d, kind, width, err := finsMemoryDescriptor(p.wire[12:16], false)
		if err != nil {
			return err
		}
		count := int(finsBE(p.wire[16:18]))
		if count > 999 {
			return finsErr(ErrUnsupportedFeature, "selected consecutive read exceeds999 items")
		}
		if len(data) != count*width {
			return finsErr(ErrMalformedMessage, "memory read response count/width")
		}
		if count > b.MaxCollectionElements-32 {
			return finsErr(ErrResourceExceeded, "memory element projection count")
		}
		values := make([]any, count)
		for i := range values {
			if kind == "bit" {
				if data[i] > 1 {
					return finsErr(ErrMalformedMessage, "plain memory bit must be00/01")
				}
				values[i] = data[i] != 0
			} else {
				values[i] = finsBE(data[2*i : 2*i+2])
			}
		}
		f["memory_area"], f["word_address"], f["bit_address"] = d["memory_area"], d["word_address"], d["bit_address"]
		if kind == "word" {
			f["word_count"], f["words"] = uint64(count), values
		} else {
			f["item_count"], f["element_type"], f["bits"] = uint64(count), kind, values
		}
		return nil
	}
	n := (len(p.wire) - 12) / 4
	if n > (b.MaxCollectionElements-32)/8 {
		return finsErr(ErrResourceExceeded, "multiple read value projection")
	}
	items := make([]any, 0, n)
	pos := 0
	for at := 12; at < len(p.wire); at += 4 {
		d, kind, width, err := finsMemoryDescriptor(p.wire[at:at+4], true)
		if err != nil {
			return err
		}
		if len(data)-pos < width+1 || data[pos] != p.wire[at] {
			return finsErr(ErrMalformedMessage, "multiple read ordered area/width differs from request")
		}
		raw := data[pos+1 : pos+1+width]
		d["data_hex"] = finsHex(raw)
		switch kind {
		case "bit":
			if raw[0] > 1 {
				return finsErr(ErrMalformedMessage, "plain memory bit must be00/01")
			}
			d["value"] = raw[0] != 0
		case "word":
			d["value"] = finsBE(raw)
		case "forced-bit":
			d["value"], d["forced"], d["undefined_bits"] = raw[0]&1 != 0, raw[0]&2 != 0, uint64(raw[0]&0xfc)
		case "forced-word":
			d["forced_mask"], d["value"] = finsBE(raw[:2]), finsBE(raw[2:])
		}
		items = append(items, d)
		pos += width + 1
	}
	if pos != len(data) {
		return finsErr(ErrMalformedMessage, "multiple read trailing data")
	}
	f["item_count"], f["memory_items"] = uint64(n), items
	return nil
}
