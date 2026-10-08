package stream_parser

import "fmt"

// Scratch holds only names and already owned values in direct decoder mode.
// It never escapes in a public result.
type structuredFieldValue struct {
	name  string
	value any
}

// StructuredDecoder projects an exact registered bridge's fields and metadata.
// Returned values own their bytes and may outlive or be modified independently
// of the input and subsequent calls. A decoder is safe for concurrent use.
type StructuredDecoder func([]byte) (any, map[string]any, error)

// StructuredDecoderForProgram recognizes only the same complete programs as
// the Node bridge. The caller must also validate the enclosing rule and parser.
// It never recognizes arbitrary Yak expressions or partial source matches.
func StructuredDecoderForProgram(source string) StructuredDecoder {
	program, ok := nativeBridgePrograms[source]
	if !ok {
		return structuredNativeDecoder(source)
	}
	return func(wire []byte) (any, map[string]any, error) {
		arena := acquireFieldArena()
		defer arena.release()
		arena.outputReserve = min(4096, 2*len(wire))
		fields, metadata, err := program.decode(wire, arena)
		if err != nil {
			return nil, nil, err
		}
		if arena.structuredValue != nil {
			return arena.structuredValue, metadata, nil
		}
		value, err := projectStructuredFieldsWithArena(wire, fields, false, "big", arena)
		if err != nil {
			return nil, nil, err
		}
		return value, metadata, nil
	}
}

func projectStructuredFields(wire []byte, fields []tlsCertificateField, list bool, endian string) (any, error) {
	return projectStructuredFieldsWithArena(wire, fields, list, endian, nil)
}

func projectStructuredFieldsWithArena(wire []byte, fields []tlsCertificateField, list bool, endian string, arena *fieldArena) (any, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	if list {
		values := make([]any, 0, len(fields))
		for i := range fields {
			value, err := projectStructuredField(wire, &fields[i], endian, arena)
			if err != nil {
				return nil, err
			}
			if value != nil {
				values = append(values, value)
			}
		}
		if len(values) == 0 {
			return nil, nil
		}
		return values, nil
	}
	values := make(map[string]any, len(fields))
	for i := range fields {
		field := &fields[i]
		value, err := projectStructuredField(wire, field, endian, arena)
		if err != nil {
			return nil, err
		}
		if value != nil {
			values[field.Name] = value
		}
	}
	if len(values) == 0 {
		return nil, nil
	}
	return values, nil
}

func projectStructuredField(wire []byte, field *tlsCertificateField, parentEndian string, arena *fieldArena) (any, error) {
	if field.Start < 0 || field.End < field.Start || field.End > len(wire) {
		return nil, fmt.Errorf("structured field %q: invalid byte span", field.Name)
	}
	endian := field.Endian
	if endian == "" {
		endian = parentEndian
	}
	if endian != "big" && endian != "little" {
		return nil, fmt.Errorf("structured field %q: unsupported byte order", field.Name)
	}
	if field.Type == "" {
		if field.List && len(field.Children) == 0 && field.Start == field.End {
			return []any{}, nil // An observed empty list is not an absent value.
		}
		return projectStructuredFieldsWithArena(wire, field.Children, field.List, endian, arena)
	}
	if len(field.Children) != 0 {
		return nil, fmt.Errorf("structured field %q: terminal with children", field.Name)
	}
	value := wire[field.Start:field.End]
	switch field.Type {
	case "string", "bytes":
		return string(value), nil
	case "raw":
		if len(value) == 0 {
			return []byte{}, nil // Preserve non-nil, zero-width raw values.
		}
		return arena.cloneBytes(value), nil
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return ConvertToVar(value, uint64(len(value)), endian, field.Type), nil
	default:
		return nil, fmt.Errorf("structured field %q: unsupported type %q", field.Name, field.Type)
	}
}
