package binx

var Exports = map[string]any{
	"Read": BinaryRead,
	"Find": FindResultByIdentifier,

	// PartDescriptor
	"toInt":    NewInt64,
	"toInt8":   NewInt8,
	"toInt16":  NewInt16,
	"toInt32":  NewInt32,
	"toInt64":  NewInt64,
	"toUint":   NewUint64,
	"toUint8":  NewUint8,
	"toUint16": NewUint16,
	"toUint32": NewUint32,
	"toUint64": NewUint64,
	"toBytes":  NewBytes,
	"toRaw":    NewBytes,
	"toBool":   NewBool,

	"toList":   NewListDescriptor,
	"toStruct": NewStructDescriptor,
}
