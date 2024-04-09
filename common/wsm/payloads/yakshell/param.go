package yakshell

type Param struct {
	Map  map[string]interface{}
	Size int
}

func NewParameter() *Param {
	return &Param{
		Map:  make(map[string]interface{}, 2),
		Size: 0,
	}
}

func (p *Param) addParam(key string, value interface{}) {
	p.Map[key] = value
	p.Size++
}

func (p *Param) AddByteParam(key string, value []byte) {
	p.addParam(key, string(value))
}
