package yakshell

import (
	"encoding/base64"
	"fmt"
	"strings"
)

type Param map[string]string

// Serialize 当是session mode的时候进行使用
func (p Param) Serialize() string {
	var result string
	for key, value := range p {
		result += fmt.Sprintf("%v~~%v,", key, base64.StdEncoding.EncodeToString([]byte(value)))
	}
	return strings.TrimRight(result, ",")
}

//type Param struct {
//	Map map[string]interface{}
//}
//
//func NewParameter() *Param {
//	return &Param{
//		Map: make(map[string]interface{}, 2),
//	}
//}
//
//func (p *Param) addParam(key string, value interface{}) {
//	p.Map[key] = value
//}
//
//func (p *Param) AddByteParam(key string, value []byte) {
//	p.addParam(key, string(value))
//}
//
//func (p *Param) Serialize() string {
//}
