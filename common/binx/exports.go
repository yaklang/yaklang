package binx

var Exports = map[string]any{
	"Read": BinaryRead,

	// PartDescriptor
	"int":    NewInt64,
	"int8":   NewInt8,
	"int16":  NewInt16,
	"int32":  NewInt32,
	"int64":  NewInt64,
	"uint":   NewUint64,
	"uint8":  NewUint8,
	"uint16": NewUint16,
	"uint32": NewUint32,
	"uint64": NewUint64,
	"bytes":  NewBytes,
	"raw":    NewBytes,
	"bool":   NewBool,
}
