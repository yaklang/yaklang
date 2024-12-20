package payload

type Item struct {
	IsRelative bool
	Offset     int
	Pos        int
	Len        int
	Content    []byte
	NoCase     bool
}

func NewItem() *Item {
	return &Item{}
}
