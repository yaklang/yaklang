package xmlrecord

// The former general-purpose layout inspector is retained only as a test
// oracle. Production accepts a fixed POM record schema, never arbitrary DTOs.
import (
	"context"
	"encoding"
	"encoding/xml"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"io"
	"reflect"
	"strings"
	"sync"
)

var (
	unmarshalerType     = reflect.TypeOf((*xml.Unmarshaler)(nil)).Elem()
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	xmlNameType         = reflect.TypeOf(xml.Name{})
	chargedMu           sync.Mutex
	chargedLayout       = map[reflect.Type]struct{}{}
)

// ChargeUnmarshalerLayout records that UnmarshalXML on t only fills t's own
// fields or map[string]string entries. Charging still walks those fields.
func ChargeUnmarshalerLayout(t reflect.Type) {
	if t == nil {
		return
	}
	chargedMu.Lock()
	chargedLayout[t] = struct{}{}
	if t.Kind() != reflect.Pointer {
		chargedLayout[reflect.PointerTo(t)] = struct{}{}
	} else {
		chargedLayout[t.Elem()] = struct{}{}
	}
	chargedMu.Unlock()
}

func layoutCharged(t reflect.Type) bool {
	chargedMu.Lock()
	defer chargedMu.Unlock()
	_, ok := chargedLayout[t]
	return ok
}

func implements(t, iface reflect.Type) bool {
	if t.Implements(iface) {
		return true
	}
	if t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(iface) {
		return true
	}
	return false
}

func checkCustomDecode(t reflect.Type) error {
	if implements(t, textUnmarshalerType) {
		return fmt.Errorf("unsupported_syntax: XML UnmarshalText")
	}
	if implements(t, unmarshalerType) && !layoutCharged(t) {
		return fmt.Errorf("unsupported_syntax: XML Unmarshaler")
	}
	return nil
}

func inspectDest(out any) (*destPlan, error) {
	if out == nil {
		return nil, fmt.Errorf("unsupported_syntax: XML destination")
	}
	v := reflect.ValueOf(out)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return nil, fmt.Errorf("unsupported_syntax: XML destination")
	}
	plan := &destPlan{}
	return plan, walkRoot(v.Elem().Type(), plan, 0)
}

func walkRoot(t reflect.Type, plan *destPlan, depth int) error {
	if err := checkCustomDecode(t); err != nil {
		return err
	}
	star := []pathElem{{Local: "*"}}
	switch t.Kind() {
	case reflect.Struct:
		return walkStruct(t, nil, plan, depth)
	case reflect.Slice:
		store, heap, err := elementCosts(t.Elem(), depth+1)
		if err != nil {
			return err
		}
		plan.slices = append(plan.slices, destSlot{path: star, store: store, heap: heap})
		return walkElemSlots(t.Elem(), nil, plan, depth+1)
	case reflect.Pointer:
		cost, err := allocCost(t.Elem(), depth+1)
		if err != nil {
			return err
		}
		plan.once = append(plan.once, destSlot{path: star, heap: cost})
		return walkRoot(t.Elem(), plan, depth+1)
	default:
		return rejectKind(t)
	}
}

func walkStruct(t reflect.Type, path []pathElem, plan *destPlan, depth int) error {
	if depth > 32 {
		return fmt.Errorf("unsupported_syntax: XML destination depth")
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return rejectKind(t)
	}
	if err := checkCustomDecode(t); err != nil {
		return err
	}
	if t == xmlNameType {
		return nil
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() && !f.Anonymous {
			continue
		}
		info := parseXMLField(f)
		ft := f.Type
		if f.Anonymous && info.inline {
			if ft.Kind() == reflect.Pointer {
				return fmt.Errorf("unsupported_syntax: XML anonymous pointer embed")
			}
			if ft.Kind() == reflect.Struct {
				if err := walkStruct(ft, path, plan, depth+1); err != nil {
					return err
				}
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if err := checkCustomDecode(ft); err != nil {
			return err
		}
		fieldPath := path
		if !info.skip && info.name != "" {
			fieldPath = appendXMLPath(path, info.xmlns, info.name)
		}
		switch ft.Kind() {
		case reflect.Slice:
			if ft.Elem().Kind() == reflect.Uint8 {
				continue
			}
			store, heap, err := elementCosts(ft.Elem(), depth+1)
			if err != nil {
				return err
			}
			if !info.skip && len(fieldPath) > 0 {
				plan.slices = append(plan.slices, destSlot{path: fieldPath, store: store, heap: heap})
			}
			if err := walkElemSlots(ft.Elem(), fieldPath, plan, depth+1); err != nil {
				return err
			}
		case reflect.Pointer:
			cost, err := allocCost(ft.Elem(), depth+1)
			if err != nil {
				return err
			}
			if !info.skip && len(fieldPath) > 0 {
				plan.once = append(plan.once, destSlot{path: fieldPath, heap: cost})
			}
			if err := walkElemSlots(ft.Elem(), fieldPath, plan, depth+1); err != nil {
				return err
			}
		case reflect.Struct:
			childPath := path
			if !info.skip && info.name != "" && !info.leaf {
				childPath = appendXMLPath(path, info.xmlns, info.name)
			}
			if err := walkStruct(ft, childPath, plan, depth+1); err != nil {
				return err
			}
		case reflect.Map:
			if !layoutCharged(ft) || ft.Key().Kind() != reflect.String || ft.Elem().Kind() != reflect.String {
				return fmt.Errorf("unsupported_syntax: XML map destination")
			}
			entry, err := budget.SizeAdd(budget.SizeMap, budget.SizePtr, budget.SizeObject, budget.SizeString, budget.SizeString)
			if err != nil {
				return err
			}
			plan.properties = append(plan.properties, destSlot{path: fieldPath, heap: entry})
		case reflect.Array:
			if err := walkElemSlots(ft.Elem(), path, plan, depth+1); err != nil {
				return err
			}
		default:
			if err := rejectKind(ft); err != nil {
				return err
			}
		}
	}
	return nil
}

func walkElemSlots(t reflect.Type, path []pathElem, plan *destPlan, depth int) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
		depth++
		if depth > 32 {
			return fmt.Errorf("unsupported_syntax: XML destination depth")
		}
	}
	if t.Kind() == reflect.Struct {
		return walkStruct(t, path, plan, depth)
	}
	if err := checkCustomDecode(t); err != nil {
		return err
	}
	return rejectKind(t)
}

type xmlField struct {
	xmlns, name string
	skip, leaf  bool
	inline      bool
}

func parseXMLField(f reflect.StructField) xmlField {
	tag := f.Tag.Get("xml")
	if tag == "-" {
		return xmlField{skip: true}
	}
	if f.Name == "XMLName" {
		return xmlField{skip: true}
	}
	xmlns := ""
	if ns, rest, ok := strings.Cut(tag, " "); ok {
		xmlns, tag = ns, rest
	}
	name, opt, _ := strings.Cut(tag, ",")
	leaf := opt == "chardata" || opt == "innerxml" || opt == "comment" || opt == "attr"
	if name == "-" {
		return xmlField{skip: true}
	}
	if tag == "" && f.Anonymous {
		return xmlField{inline: true}
	}
	if name == "" {
		if leaf {
			return xmlField{skip: true, leaf: true}
		}
		return xmlField{name: f.Name}
	}
	return xmlField{xmlns: xmlns, name: name, leaf: leaf}
}

func appendXMLPath(path []pathElem, xmlns, name string) []pathElem {
	parts := strings.Split(name, ">")
	out := append([]pathElem(nil), path...)
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		el := pathElem{Local: p}
		if i == len(parts)-1 {
			el.Space = xmlns
		}
		out = append(out, el)
	}
	return out
}

func elementCosts(t reflect.Type, depth int) (store, heap int64, err error) {
	if err = checkCustomDecode(t); err != nil {
		return 0, 0, err
	}
	if t.Kind() == reflect.Pointer {
		store = int64(t.Size())
		heap, err = allocCost(t.Elem(), depth)
		return store, heap, err
	}
	if err = rejectKind(t); err != nil && t.Kind() != reflect.Struct && t.Kind() != reflect.Array {
		return 0, 0, err
	}
	store = int64(t.Size())
	heap, err = nestedHeap(t, depth)
	return store, heap, err
}

func nestedHeap(t reflect.Type, depth int) (int64, error) {
	if t.Kind() != reflect.Struct {
		return 0, nil
	}
	var heap int64
	for i := 0; i < t.NumField(); i++ {
		ft := t.Field(i).Type
		if ft.Kind() == reflect.Struct {
			c, err := nestedHeap(ft, depth+1)
			if err != nil {
				return 0, err
			}
			heap, err = budget.SizeAdd(heap, c)
			if err != nil {
				return 0, err
			}
		}
	}
	return heap, nil
}

func allocCost(t reflect.Type, depth int) (int64, error) {
	if depth > 32 {
		return 0, fmt.Errorf("unsupported_syntax: XML destination depth")
	}
	if err := checkCustomDecode(t); err != nil {
		return 0, err
	}
	if err := rejectKind(t); err != nil && t.Kind() != reflect.Struct && t.Kind() != reflect.Pointer && t.Kind() != reflect.Array {
		return 0, err
	}
	base, err := budget.SizeAdd(int64(t.Size()), budget.SizeObject)
	if err != nil {
		return 0, err
	}
	if t.Kind() == reflect.Pointer {
		inner, err := allocCost(t.Elem(), depth+1)
		if err != nil {
			return 0, err
		}
		return budget.SizeAdd(base, inner)
	}
	extra, err := nestedHeap(t, depth+1)
	if err != nil {
		return 0, err
	}
	return budget.SizeAdd(base, extra)
}

func rejectKind(t reflect.Type) error {
	switch t.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.String, reflect.Struct, reflect.Array:
		return nil
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return nil
		}
		return fmt.Errorf("unsupported_syntax: XML destination slice")
	case reflect.Map:
		if layoutCharged(t) && t.Key().Kind() == reflect.String && t.Elem().Kind() == reflect.String {
			return nil
		}
		return fmt.Errorf("unsupported_syntax: XML map destination")
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.UnsafePointer, reflect.Invalid:
		return fmt.Errorf("unsupported_syntax: XML destination %s", t.Kind())
	default:
		return nil
	}
}

func decodeTest(ctx context.Context, r io.Reader, out any) error {
	plan, err := inspectDest(out)
	if err != nil {
		return err
	}
	return decodeWithPlan(ctx, r, out, plan)
}

func pathKeySize(stack []xml.Name) int64 {
	var n int64
	for i, el := range stack {
		if i > 0 {
			n++
		}
		n += int64(len(el.Space)) + 1 + int64(len(el.Local))
	}
	return n
}

func writePathScratch(dst []byte, stack []xml.Name) []byte {
	dst = dst[:0]
	for i, el := range stack {
		if i > 0 {
			dst = append(dst, 0x1e)
		}
		dst = append(dst, el.Space...)
		dst = append(dst, 0x1f)
		dst = append(dst, el.Local...)
	}
	return dst
}

func ensurePathScratch(st *budget.State, buf *[]byte, charged *int64, want int64) error {
	if want <= int64(cap(*buf)) {
		return nil
	}
	newCap := nextSliceCap(want, int64(cap(*buf)))
	if _, err := budget.SizeAdd(newCap); err != nil {
		return err
	}
	if err := reserveCap(st, charged, newCap); err != nil {
		return err
	}
	*buf = make([]byte, 0, int(newCap))
	return nil
}

func reserveCap(st *budget.State, charged *int64, want int64) error {
	if want <= *charged {
		return nil
	}
	if err := st.Result(want - *charged); err != nil {
		return err
	}
	*charged = want
	return nil
}

func reserveNameStack(st *budget.State, stack []xml.Name, charged *int64) error {
	needLen := int64(len(stack) + 1)
	oldCap := int64(cap(stack))
	if needLen <= oldCap {
		return nil
	}
	newCap := nextSliceCap(needLen, oldCap)
	delta := newCap - *charged
	need, err := budget.SizeMul(int(delta), budget.SizeObject)
	if err != nil {
		return err
	}
	if *charged == 0 {
		need, err = budget.SizeAdd(need, budget.SizeSlice)
		if err != nil {
			return err
		}
	}
	if err := st.Result(need); err != nil {
		return err
	}
	*charged = newCap
	return nil
}

type pathHit struct {
	stack []xml.Name
	n     int64
}

func countStarts(ctx context.Context, d *xml.Decoder, l budget.Limits) (func([]pathElem) int64, int64, error) {
	guard := &tokens{ctx: ctx, d: d, l: l}
	st := budget.From(ctx)
	index := map[string]int{}
	var hits []pathHit
	var stack []xml.Name
	var stackCharged int64
	var scratch []byte
	var keyCharged int64
	var props int64
	var inProps int
	for {
		tok, err := guard.Token()
		if err == io.EOF {
			return func(path []pathElem) int64 {
				var n int64
				for _, h := range hits {
					if pathMatch(h.stack, path) {
						n += h.n
					}
				}
				return n
			}, props, nil
		}
		if err != nil {
			return nil, 0, err
		}
		switch v := tok.(type) {
		case xml.StartElement:
			if err := reserveNameStack(st, stack, &stackCharged); err != nil {
				return nil, 0, err
			}
			stack = append(stack, v.Name)
			keyBytes := pathKeySize(stack)
			if err := ensurePathScratch(st, &scratch, &keyCharged, keyBytes); err != nil {
				return nil, 0, err
			}
			scratch = writePathScratch(scratch, stack)
			if i, ok := index[string(scratch)]; ok {
				hits[i].n++
			} else {
				need, err := budget.SizeAdd(budget.SizeSlice, int64(len(stack))*budget.SizeObject)
				if err != nil {
					return nil, 0, err
				}
				if err := st.Result(need); err != nil {
					return nil, 0, err
				}
				storedNeed, err := budget.SizeAdd(budget.SizeString, keyBytes, budget.SizePtr)
				if err != nil {
					return nil, 0, err
				}
				if err := st.Result(storedNeed); err != nil {
					return nil, 0, err
				}
				stored := string(scratch)
				cp := append([]xml.Name(nil), stack...)
				index[stored] = len(hits)
				hits = append(hits, pathHit{stack: cp, n: 1})
			}
			if inProps > 0 {
				props++
				// properties.UnmarshalXML keys Local in any Space. This Result of
				// the key string overlaps destination entry SizeString and is extra.
				if err := st.Result(budget.SizeOfString(v.Name.Local)); err != nil {
					return nil, 0, err
				}
			}
			if v.Name.Local == "properties" {
				inProps++
			}
		case xml.EndElement:
			if v.Name.Local == "properties" && inProps > 0 {
				inProps--
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
}
