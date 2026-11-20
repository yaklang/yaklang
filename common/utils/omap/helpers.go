package omap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/gobwas/glob"
	"github.com/segmentio/ksuid"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/utils"
)

type jsonFetcherMarshaler interface {
	MarshalJSONWithKeyValueFetcher(k func(t any) ([]byte, error), vf func(any) ([]byte, error)) ([]byte, error)
}

type parsedJSONEntry struct {
	keyRaw string
	valRaw string
}

type parsedJSONResult struct {
	kind    uint8 // 0=none, 1=array, 2=object, 3=literal
	entries []parsedJSONEntry
	literal any
}

func newNonNamedKey() string {
	return ksuid.New().String() + nonnamedkeyconst
}

func marshalOrderedMapJSON(
	haveLiteral bool,
	literalValue any,
	canAsList bool,
	keys []any,
	vals []any,
	k func(t any) ([]byte, error),
	vf func(any) ([]byte, error),
) ([]byte, error) {
	if haveLiteral {
		raw, err := json.Marshal(literalValue)
		if err != nil {
			return nil, utils.Errorf("cannot marshal literal val: %T", literalValue)
		}
		return raw, nil
	}
	if canAsList {
		var buf bytes.Buffer
		buf.WriteByte('[')
		for index, val := range vals {
			if index != 0 {
				buf.WriteByte(',')
			}
			var raw []byte
			var err error
			if vf != nil {
				raw, err = vf(val)
			} else if vIns, ok := val.(jsonFetcherMarshaler); ok {
				raw, err = vIns.MarshalJSONWithKeyValueFetcher(k, vf)
			} else {
				raw, err = json.Marshal(val)
			}
			if err != nil {
				buf.WriteString("null")
				continue
			}
			buf.Write(raw)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	}

	var buf bytes.Buffer
	buf.WriteByte('{')
	for idx := range keys {
		if idx != 0 {
			buf.WriteByte(',')
		}
		var raw []byte
		var err error
		if k != nil {
			raw, err = k(keys[idx])
		} else {
			raw, err = json.Marshal(keys[idx])
		}
		buf.Write(raw)
		buf.WriteByte(':')

		val := vals[idx]
		if vf != nil {
			raw, err = vf(val)
		} else if vIns, ok := val.(jsonFetcherMarshaler); ok {
			raw, err = vIns.MarshalJSONWithKeyValueFetcher(k, vf)
		} else {
			raw, err = json.Marshal(val)
		}
		if err != nil {
			buf.WriteString("null")
			continue
		}
		buf.Write(raw)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func parseOrderedMapJSON(raw []byte) parsedJSONResult {
	gresult := gjson.ParseBytes(raw)
	if gresult.IsArray() {
		arr := gresult.Array()
		entries := make([]parsedJSONEntry, 0, len(arr))
		for _, r := range arr {
			entries = append(entries, parsedJSONEntry{valRaw: r.Raw})
		}
		return parsedJSONResult{kind: 1, entries: entries}
	}
	if gresult.IsObject() {
		var entries []parsedJSONEntry
		gresult.ForEach(func(key, value gjson.Result) bool {
			entries = append(entries, parsedJSONEntry{keyRaw: key.Raw, valRaw: value.Raw})
			return true
		})
		return parsedJSONResult{kind: 2, entries: entries}
	}
	if gresult.IsBool() {
		return parsedJSONResult{kind: 3, literal: gresult.Bool()}
	}
	return parsedJSONResult{kind: 3, literal: gresult.Num}
}

func formatOrderedMapString(keys, vals []any) string {
	var builder strings.Builder
	builder.WriteByte('{')
	for i := range keys {
		builder.WriteString(fmt.Sprintf("%v: %#v", keys[i], vals[i]))
		if i != len(keys)-1 {
			builder.WriteString(", ")
		}
	}
	builder.WriteByte('}')
	return builder.String()
}

func matchExactStrings(keys []any, targets []string) []int {
	var matched []int
	for idx, key := range keys {
		ks := utils.InterfaceToString(key)
		for _, j := range targets {
			if ks == j {
				matched = append(matched, idx)
				break
			}
		}
	}
	return matched
}

func matchIndexSet(n int, indices []int) []int {
	indexMap := make(map[int]struct{}, len(indices))
	for _, idx := range indices {
		indexMap[idx] = struct{}{}
	}
	var matched []int
	for idx := 0; idx < n; idx++ {
		if _, ok := indexMap[idx]; ok {
			matched = append(matched, idx)
		}
	}
	return matched
}

func matchRegexKeys(keys []any, pattern string) ([]int, error) {
	rule, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	var matched []int
	for idx, key := range keys {
		if rule.MatchString(utils.InterfaceToString(key)) {
			matched = append(matched, idx)
		}
	}
	return matched, nil
}

func matchGlobKeys(keys []any, pattern string, seps []string) ([]int, error) {
	sepsChar := []rune(strings.Join(seps, ""))
	rule, err := glob.Compile(pattern, sepsChar...)
	if err != nil {
		return nil, err
	}
	var matched []int
	for idx, key := range keys {
		if rule.Match(utils.InterfaceToString(key)) {
			matched = append(matched, idx)
		}
	}
	return matched, nil
}

func walkMatchRegex(root any, pattern string) ([]any, error) {
	rule, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	var matched []any
	Walk(root, func(parent any, key any, value any) bool {
		if rule.MatchString(utils.InterfaceToString(key)) {
			matched = append(matched, value)
		}
		return true
	})
	return matched, nil
}

func walkMatchGlob(root any, pattern string, seps []string) ([]any, error) {
	sepsChar := []rune(strings.Join(seps, ""))
	rule, err := glob.Compile(pattern, sepsChar...)
	if err != nil {
		return nil, err
	}
	var matched []any
	Walk(root, func(parent any, key any, value any) bool {
		if rule.Match(utils.InterfaceToString(key)) {
			matched = append(matched, value)
		}
		return true
	})
	return matched, nil
}

func containsDeepEqual(vals []any, target any) bool {
	for _, v := range vals {
		if reflect.DeepEqual(target, v) {
			return true
		}
	}
	return false
}

type flatValueEntry struct {
	named bool
	key   string
	val   any
}

func flattenValuesForMap(vals []any) []flatValueEntry {
	var out []flatValueEntry
	for _, v := range vals {
		vv := reflect.ValueOf(v)
		vt := reflect.TypeOf(v)
		switch vt.Kind() {
		case reflect.Map:
			for _, key := range vv.MapKeys() {
				out = append(out, flatValueEntry{
					named: true,
					key:   utils.InterfaceToString(key.Interface()),
					val:   vv.MapIndex(key).Interface(),
				})
			}
		case reflect.Array, reflect.Slice:
			for i := 0; i < vv.Len(); i++ {
				out = append(out, flatValueEntry{val: vv.Index(i).Interface()})
			}
		default:
			out = append(out, flatValueEntry{val: v})
		}
	}
	return out
}

func extractGeneralMapEntries(m any) []flatValueEntry {
	if m == nil {
		return nil
	}
	for {
		ty := reflect.TypeOf(m)
		if ty.Kind() != reflect.Ptr {
			break
		}
		m = reflect.ValueOf(m).Elem().Interface()
		if m == nil {
			return nil
		}
	}
	ty := reflect.TypeOf(m)
	switch ty.Kind() {
	case reflect.Map:
		vv := reflect.ValueOf(m)
		keys := vv.MapKeys()
		out := make([]flatValueEntry, 0, len(keys))
		for _, key := range keys {
			out = append(out, flatValueEntry{
				named: true,
				key:   utils.InterfaceToString(key.Interface()),
				val:   vv.MapIndex(key).Interface(),
			})
		}
		return out
	case reflect.Array, reflect.Slice:
		vv := reflect.ValueOf(m)
		n := vv.Len()
		out := make([]flatValueEntry, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, flatValueEntry{val: vv.Index(i).Interface()})
		}
		return out
	case reflect.Struct:
		vv := reflect.ValueOf(m)
		n := vv.NumField()
		out := make([]flatValueEntry, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, flatValueEntry{
				named: true,
				key:   vv.Type().Field(i).Name,
				val:   vv.Field(i).Interface(),
			})
		}
		return out
	default:
		return []flatValueEntry{{
			named: true,
			key:   utils.InterfaceToString(m),
			val:   m,
		}}
	}
}
