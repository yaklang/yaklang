package base

// Only immutable primitive defaults are shared. A mutable value, custom
// parser/unit, missing endian, or an unknown type keeps ordinary assignments.
// Every later overwrite remains a private log version and every public
// exposure materializes an independent journal with the original write order.
type configPrefix struct {
	writes    [5]compactConfigWrite
	count     uint16
	positions [32]uint16
	order     [32]uint8
	orderLen  uint8
}

var configPrefixTypes = [...]string{"", "raw", "string", "uint8", "uint16", "uint32", "uint64", "uint", "int8", "int16", "int32", "int64", "int", "bool"}
var configPrefixes = func() [2][2][len(configPrefixTypes)]configPrefix {
	var result [2][2][len(configPrefixTypes)]configPrefix
	for endian := 0; endian < 2; endian++ {
		for unit := 0; unit < 2; unit++ {
			for typ, name := range configPrefixTypes {
				p := &result[endian][unit][typ]
				add := func(k uint8, v any) {
					p.writes[p.count] = compactConfigWrite{v, k, true}
					p.count++
					p.positions[k] = p.count
					p.order[p.orderLen] = k
					p.orderLen++
					if p.positions[1] == 0 {
						p.positions[1] = 1
						p.order[p.orderLen] = 1
						p.orderLen++
					}
				}
				add(2, []string{"big", "little"}[endian])
				add(3, "default")
				if unit == 1 {
					add(4, "byte")
				}
				if typ > 0 {
					add(5, true)
					add(6, name)
				}
			}
		}
	}
	return result
}()

func (s *configStore) initializePrefix(inherited [3]configEntry, count int, items []ConfigItem) (skip int, used bool) {
	if count < 2 || inherited[0].key != "endian" || inherited[1].key != "parser" || inherited[1].value != "default" {
		return 0, false
	}
	endian := 0
	if inherited[0].value == "little" {
		endian = 1
	} else if inherited[0].value != "big" {
		return 0, false
	}
	unit := 0
	if count == 3 {
		if inherited[2].key != "unit" || inherited[2].value != "byte" {
			return 0, false
		}
		unit = 1
	}
	typ := 0
	if len(items) >= 2 && items[0].Key == CfgIsTerminal && items[0].Value == true && items[1].Key == CfgType {
		if name, ok := items[1].Value.(string); ok {
			for i, v := range configPrefixTypes {
				if i > 0 && v == name {
					typ = i
					skip = 2
					break
				}
			}
		}
	}
	p := &configPrefixes[endian][unit][typ]
	s.prefix = p
	s.positions = p.positions
	s.order = p.order
	s.orderLen = p.orderLen
	s.historyCount = p.count
	return skip, true
}

func (s *configStore) compactWriteCount() int {
	if s.prefix != nil {
		return int(s.prefix.count) + len(s.writes)
	}
	return len(s.writes)
}

func (s *configStore) compactWrite(index int) compactConfigWrite {
	if s.prefix != nil {
		if index < int(s.prefix.count) {
			return s.prefix.writes[index]
		}
		index -= int(s.prefix.count)
	}
	return s.writes[index]
}
