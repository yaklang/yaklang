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
