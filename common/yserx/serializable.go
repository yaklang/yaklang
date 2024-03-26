package yserx

type JavaSerializable interface {
	//String() string
	//SDumper(indent int) string
	Marshal(*MarshalContext) []byte
}

type MarshalContext struct {
	DirtyDataLength int
	StringCharLength int
}
