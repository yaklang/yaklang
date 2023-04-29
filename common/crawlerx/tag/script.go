package tag

type DoStringScript struct {
	data interface{}
	call string
}

func (script *DoStringScript) Call() bool {
	return true
}
