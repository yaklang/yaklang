package yserx

type JavaSerializable interface {
	//String() string
	//SDumper(indent int) string
	Marshal() []byte
}
