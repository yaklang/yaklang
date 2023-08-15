package ssa

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/samber/lo"
	"golang.org/x/exp/slices"
)

type BasicKind types.BasicKind

const (
	Invalid BasicKind = iota // type is invalid

	// predeclared types
	Bool
	Int
	Int8
	Int16
	Int32
	Int64
	Uint
	Uint8
	Uint16
	Uint32
	Uint64
	Uintptr
	Float32
	Float64
	Complex64
	Complex128
	String
	UnsafePointer

	// types for untyped values
	UntypedBool
	UntypedInt
	UntypedRune
	UntypedFloat
	UntypedComplex
	UntypedString
	UntypedNil

	// aliases
	Byte = Uint8
	Rune = Int32
)

var (
	basicTypesKind = make(map[BasicKind]*types.Basic)
	basicTypesStr  = make(map[string]*types.Basic)
)

func init() {
	for _, basic := range types.Typ {
		basicTypesKind[BasicKind(basic.Kind())] = basic
		basicTypesStr[basic.String()] = basic
	}
	basicTypesKind[Int] = basicTypesKind[Int64]
	basicTypesStr["int"] = basicTypesStr["int64"]
}

type Type interface {
	String() string
}
type Types []Type // each value can have multiple type possible

// ==================== slice type
type SliceType struct {
	Elem Types
}

func (i SliceType) String() string {
	return "[]" + i.Elem.String()
}

func NewSliceType(elem Types) *SliceType {
	return &SliceType{
		Elem: elem,
	}
}

var _ (Type) = (*SliceType)(nil)

// ==================== struct type
type StructType struct {
	Key   []Value
	Field []Types
}

func (s StructType) String() string {
	str := ""
	for i := range s.Key {
		str += fmt.Sprintf("<%s> %s, ", s.Field[i].String(), s.Key[i])
	}
	return "struct {" + str + "}"
}

func NewStructType() *StructType {
	return &StructType{
		Key:   make([]Value, 0),
		Field: make([]Types, 0),
	}
}

func (s *StructType) AddField(key Value, field Types) {
	s.Key = append(s.Key, key)
	s.Field = append(s.Field, field)
}

func (s *StructType) GetField(key Value) Types {
	if index := slices.Index(s.Key, key); index != -1 {
		return s.Field[index]
	}
	return nil
}

var _ (Type) = (*StructType)(nil)

// ==================== map type

type MapType struct {
	Key, Value Types
}

func (m MapType) String() string {
	return fmt.Sprintf("map[%s]%s", m.Key.String(), m.Value.String())
}

func NewMapType(key, value Types) *MapType {
	return &MapType{
		Key:   key,
		Value: value,
	}
}

var _ (Type) = (*MapType)(nil)

// ===================== transform

func (s *StructType) Transform() Type {
	field := lo.UniqBy(s.Field, func(t Types) string { return t.String() })
	key := lo.UniqBy(s.Key, func(t Value) string { return t.GetType().String() })
	if len(field) == 1 {
		if len(key) == 1 {
			if key[0].GetType().String() == "int64" {
				return NewSliceType(field[0])
			}
			return NewMapType(key[0].GetType(), field[0])
		}
	}
	return s
}

// ====================== chan type
type ChanType struct {
	elem Types
}

func NewChanType(elem Types) *ChanType {
	return &ChanType{
		elem: elem,
	}
}

func (c ChanType) String() string {
	return fmt.Sprintf("chan %s", c.elem)
}

// return true  if org != typs
// return false if org == typs
func (org Types) Compare(typs Types) bool {
	if len(org) == 0 && len(typs) != 0 {
		return true
	}
	return slices.CompareFunc(org, typs, func(org, typ Type) int {
		if org.String() == typs.String() {
			return 0
		}
		return 1
	}) != 0
}

func (t Types) String() string {
	return strings.Join(
		lo.Map(t, func(typ Type, _ int) string {
			if typ == nil {
				return "nil"
			} else {
				return typ.String()
			}
		}),
		", ",
	)
}
