package wsm

type IResult interface {
	Unmarshal([]byte, map[string]string) error
}
