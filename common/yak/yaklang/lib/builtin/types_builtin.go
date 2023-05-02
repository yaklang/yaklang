package builtin

import (
	"reflect"
)

// -----------------------------------------------------------------------------

type tyFloat32 int

func (p tyFloat32) GoType() reflect.Type {

	return gotyFloat32
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p tyFloat32) NewInstance(args ...interface{}) interface{} {

	ret := new(float32)
	if len(args) > 0 {
		*ret = Float32(args[0])
	}
	return ret
}

func (p tyFloat32) Call(a interface{}) float32 {

	return Float32(a)
}

func (p tyFloat32) String() string {

	return "float32"
}

// TyFloat32 represents the `float32` type.
var TyFloat32 = tyFloat32(0)

// -----------------------------------------------------------------------------

type tyFloat64 int

func (p tyFloat64) GoType() reflect.Type {

	return gotyFloat64
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p tyFloat64) NewInstance(args ...interface{}) interface{} {

	ret := new(float64)
	if len(args) > 0 {
		*ret = Float64(args[0])
	}
	return ret
}

func (p tyFloat64) Call(a interface{}) float64 {

	return Float64(a)
}

func (p tyFloat64) String() string {

	return "float64"
}

// TyFloat64 represents the `float64` type.
var TyFloat64 = tyFloat64(0)

// -----------------------------------------------------------------------------

type tyInt int

func (p tyInt) GoType() reflect.Type {

	return gotyInt
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p tyInt) NewInstance(args ...interface{}) interface{} {

	ret := new(int)
	if len(args) > 0 {
		*ret = Int(args[0])
	}
	return ret
}

func (p tyInt) Call(a interface{}) int {

	return Int(a)
}

func (p tyInt) String() string {

	return "int"
}

// TyInt represents the `int` type.
var TyInt = tyInt(0)

// -----------------------------------------------------------------------------

type tyInt8 int

func (p tyInt8) GoType() reflect.Type {

	return gotyInt8
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p tyInt8) NewInstance(args ...interface{}) interface{} {

	ret := new(int8)
	if len(args) > 0 {
		*ret = Int8(args[0])
	}
	return ret
}

func (p tyInt8) Call(a interface{}) int8 {

	return Int8(a)
}

func (p tyInt8) String() string {

	return "int8"
}

// TyInt8 represents the `int8` type.
var TyInt8 = tyInt8(0)

// -----------------------------------------------------------------------------

type tyInt16 int

func (p tyInt16) GoType() reflect.Type {

	return gotyInt16
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p tyInt16) NewInstance(args ...interface{}) interface{} {

	ret := new(int16)
	if len(args) > 0 {
		*ret = Int16(args[0])
	}
	return ret
}

func (p tyInt16) Call(a interface{}) int16 {

	return Int16(a)
}

func (p tyInt16) String() string {

	return "int16"
}

// TyInt16 represents the `int16` type.
var TyInt16 = tyInt16(0)

// -----------------------------------------------------------------------------

type tyInt32 int

func (p tyInt32) GoType() reflect.Type {

	return gotyInt32
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p tyInt32) NewInstance(args ...interface{}) interface{} {

	ret := new(int32)
	if len(args) > 0 {
		*ret = Int32(args[0])
	}
	return ret
}

func (p tyInt32) Call(a interface{}) int32 {

	return Int32(a)
}

func (p tyInt32) String() string {

	return "int32"
}

// TyInt32 represents the `int32` type.
var TyInt32 = tyInt32(0)

// -----------------------------------------------------------------------------

type tyInt64 int

func (p tyInt64) GoType() reflect.Type {

	return gotyInt64
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p tyInt64) NewInstance(args ...interface{}) interface{} {

	ret := new(int64)
	if len(args) > 0 {
		*ret = Int64(args[0])
	}
	return ret
}

func (p tyInt64) Call(a interface{}) int64 {

	return Int64(a)
}

func (p tyInt64) String() string {

	return "int64"
}

// TyInt64 represents the `int64` type.
var TyInt64 = tyInt64(0)

// -----------------------------------------------------------------------------

type tyUint int

func (p tyUint) GoType() reflect.Type {

	return gotyUint
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p tyUint) NewInstance(args ...interface{}) interface{} {

	ret := new(uint)
	if len(args) > 0 {
		*ret = Uint(args[0])
	}
	return ret
}

func (p tyUint) Call(a interface{}) uint {

	return Uint(a)
}

func (p tyUint) String() string {

	return "uint"
}

// TyUint represents the `uint` type.
var TyUint = tyUint(0)

// -----------------------------------------------------------------------------

type tyUint8 int

func (p tyUint8) GoType() reflect.Type {

	return gotyUint8
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p tyUint8) NewInstance(args ...interface{}) interface{} {

	ret := new(uint8)
	if len(args) > 0 {
		*ret = Uint8(args[0])
	}
	return ret
}

func (p tyUint8) Call(a interface{}) uint8 {

	return Uint8(a)
}

func (p tyUint8) String() string {

	return "uint8"
}

// TyUint8 represents the `uint8` type.
var TyUint8 = tyUint8(0)

// -----------------------------------------------------------------------------

type tyUint16 int

func (p tyUint16) GoType() reflect.Type {

	return gotyUint16
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p tyUint16) NewInstance(args ...interface{}) interface{} {

	ret := new(uint16)
	if len(args) > 0 {
		*ret = Uint16(args[0])
	}
	return ret
}

func (p tyUint16) Call(a interface{}) uint16 {

	return Uint16(a)
}

func (p tyUint16) String() string {

	return "uint16"
}

// TyUint16 represents the `uint16` type.
var TyUint16 = tyUint16(0)

// -----------------------------------------------------------------------------

type tyUint32 int

func (p tyUint32) GoType() reflect.Type {

	return gotyUint32
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p tyUint32) NewInstance(args ...interface{}) interface{} {

	ret := new(uint32)
	if len(args) > 0 {
		*ret = Uint32(args[0])
	}
	return ret
}

func (p tyUint32) Call(a interface{}) uint32 {

	return Uint32(a)
}

func (p tyUint32) String() string {

	return "uint32"
}

// TyUint32 represents the `uint32` type.
var TyUint32 = tyUint32(0)

// -----------------------------------------------------------------------------

type tyUint64 int

func (p tyUint64) GoType() reflect.Type {

	return gotyUint64
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p tyUint64) NewInstance(args ...interface{}) interface{} {

	ret := new(uint64)
	if len(args) > 0 {
		*ret = Uint64(args[0])
	}
	return ret
}

func (p tyUint64) Call(a interface{}) uint64 {

	return Uint64(a)
}

func (p tyUint64) String() string {

	return "uint64"
}

// TyUint64 represents the `uint64` type.
var TyUint64 = tyUint64(0)

// -----------------------------------------------------------------------------

type tyString int

func (p tyString) GoType() reflect.Type {

	return gotyString
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p tyString) NewInstance(args ...interface{}) interface{} {

	ret := new(string)
	if len(args) > 0 {
		*ret = String(args[0])
	}
	return ret
}

func (p tyString) Call(a interface{}) string {
	return String(a)
}

func (p tyString) String() string {

	return "string"
}

// TyString represents the `string` type.
var TyString = tyString(0)

// -----------------------------------------------------------------------------

type tyBool int

func (p tyBool) GoType() reflect.Type {

	return gotyBool
}

// NewInstance creates a new instance of a yaklang type. required by `yaklang type` spec.
func (p tyBool) NewInstance(args ...interface{}) interface{} {

	ret := new(bool)
	if len(args) > 0 {
		*ret = Bool(args[0])
	}
	return ret
}

func (p tyBool) Call(a interface{}) bool {

	return Bool(a)
}

func (p tyBool) String() string {

	return "bool"
}

// TyBool represents the `bool` type.
var TyBool = tyBool(0)

// -----------------------------------------------------------------------------
