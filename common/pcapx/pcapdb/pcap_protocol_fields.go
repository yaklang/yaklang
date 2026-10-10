package pcapdb

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/gorm"
)

const (
	maxProtocolFieldPredicates = 16
	maxProtocolFieldIndexes    = 16
	protocolFieldIndexPrefix   = "messages_field_"
)

type protocolFieldPredicate struct {
	path      string
	value     any
	existence int // 0: typed equality; 1: present; -1: absent.
}

func (c *queryConfig) addField(field protocolFieldPredicate) error {
	if err := validateProtocolFieldPath(field.path); err != nil {
		return err
	}
	if len(c.fields) >= maxProtocolFieldPredicates {
		return fmt.Errorf("pcapdb: at most %d field predicates are allowed", maxProtocolFieldPredicates)
	}
	c.fields = append(c.fields, field)
	return nil
}

// Accept read paths, including quoted labels and fixed/from-end array indexes.
// Wildcards and append paths are not scalar lookups. Keep malformed paths from
// silently appearing valid when the candidate set happens to be empty.
func validateProtocolFieldPath(path string) error {
	invalid := func() error { return fmt.Errorf("pcapdb: invalid scalar JSON field path %q", path) }
	if len(path) == 0 || len(path) > 1024 || path[0] != '$' || !utf8.ValidString(path) {
		return invalid()
	}
	for _, char := range path {
		if char < 0x20 {
			return invalid()
		}
	}
	segments := 0
	for i := 1; i < len(path); segments++ {
		if segments >= 64 {
			return invalid()
		}
		switch path[i] {
		case '.':
			i++
			if i == len(path) {
				return invalid()
			}
			start := i
			if path[i] == '"' {
				i++
				for i < len(path) && path[i] != '"' {
					if path[i] == '\\' {
						i++
					}
					i++
				}
				if i >= len(path) {
					return invalid()
				}
				i++
				var label string
				if json.Unmarshal([]byte(path[start:i]), &label) != nil || strings.ContainsRune(label, 0) {
					return invalid()
				}
			} else {
				for i < len(path) && path[i] != '.' && path[i] != '[' {
					if path[i] == ']' || path[i] == '"' || path[i] == '*' {
						return invalid()
					}
					i++
				}
				if i == start {
					return invalid()
				}
			}
		case '[':
			i++
			fromEnd := strings.HasPrefix(path[i:], "#-")
			if fromEnd {
				i += 2
			}
			start := i
			for i < len(path) && path[i] >= '0' && path[i] <= '9' {
				i++
			}
			if i == start || i == len(path) || path[i] != ']' {
				return invalid()
			}
			index, err := strconv.ParseInt(path[start:i], 10, 64)
			if err != nil || (fromEnd && index == 0) {
				return invalid()
			}
			i++
		default:
			return invalid()
		}
	}
	return nil
}

func protocolFieldValue(value any) (any, error) {
	switch number := value.(type) {
	case nil, string, bool, int64:
		return value, nil
	case int:
		return int64(number), nil
	case int8:
		return int64(number), nil
	case int16:
		return int64(number), nil
	case int32:
		return int64(number), nil
	case uint8:
		return int64(number), nil
	case uint16:
		return int64(number), nil
	case uint32:
		return int64(number), nil
	case uint:
		if uint64(number) <= math.MaxInt64 {
			return int64(number), nil
		}
	case uint64:
		if number <= math.MaxInt64 {
			return int64(number), nil
		}
	case json.Number:
		if integer, err := number.Int64(); err == nil {
			return integer, nil
		}
		// Large integer literals must not silently become rounded REAL values.
		if strings.ContainsAny(string(number), ".eE") {
			if real, err := number.Float64(); err == nil {
				return protocolFieldValue(real)
			}
		}
	case float32:
		return protocolFieldValue(float64(number))
	case float64:
		if !math.IsNaN(number) && !math.IsInf(number, 0) {
			return number, nil
		}
	}
	return nil, fmt.Errorf("pcapdb: field value must be a finite scalar within SQLite's signed integer/REAL range")
}

// Use one expression builder for both queries and indexes. A bound JSON path
// (json_extract(fields, ?)) cannot match an index on a literal path. Paths are
// validated and SQL-quoted here; query VALUES always remain bind parameters.
func protocolFieldExpression(function, path string) string {
	return function + "(fields, '" + strings.ReplaceAll(path, "'", "''") + "')"
}

func protocolFieldScalarCondition(path string) string {
	return protocolFieldExpression("json_type", path) + " IN ('text','integer','real','true','false','null')"
}

func protocolFieldIndexName(path string) string {
	hash := sha256.Sum256([]byte(path))
	return protocolFieldIndexPrefix + hex.EncodeToString(hash[:])
}

func (field protocolFieldPredicate) apply(query *gorm.DB) *gorm.DB {
	if field.existence > 0 {
		return query.Where("json_type(fields, ?) IS NOT NULL", field.path)
	}
	if field.existence < 0 {
		return query.Where("json_type(fields, ?) IS NULL", field.path)
	}
	// This shared condition also allows SQLite to use the partial field index.
	query = query.Where(protocolFieldScalarCondition(field.path))
	kind := protocolFieldExpression("json_type", field.path)
	switch value := field.value.(type) {
	case nil:
		query = query.Where(kind + " = 'null'")
	case bool:
		if value {
			query = query.Where(kind + " = 'true'")
		} else {
			query = query.Where(kind + " = 'false'")
		}
	case string:
		query = query.Where(kind + " = 'text'")
	default:
		query = query.Where(kind + " IN ('integer','real')")
	}
	return query.Where(protocolFieldExpression("json_extract", field.path)+" IS ?", field.value)
}
