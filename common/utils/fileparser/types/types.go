package types

type File struct {
	Type       string
	BinaryData []byte
	FileName   string
	Metadata   map[string]string
}
