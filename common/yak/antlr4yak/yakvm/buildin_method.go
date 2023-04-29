package yakvm

import (
	"bytes"
	"fmt"
	"strings"
)

type buildinMethod struct {
	Name string

	// 参数表
	// 如果是几个参数，应该执行什么样的函数，内容是啥？
	ParamTable      []string
	IsVariadicParam bool
	Snippet         string

	// 执行的核心函数
	HandlerFactory MethodFactory

	// 这个内置方法的描述
	Description string
}

func (b *buildinMethod) VSCodeSnippets() (string, string) {
	if b.Snippet != "" {
		return b.Snippet, ""
	}

	/*
		${1:default}
		$d -> order
		$0 -> end
	*/
	if b.ParamTable == nil || len(b.ParamTable) <= 0 {
		d := fmt.Sprintf(`%v()`, b.Name)
		return d, d
	}

	paramsList := b.ParamTable[:]
	if b.IsVariadicParam {
		paramsList = paramsList[:len(paramsList)-1]
	}

	var buf bytes.Buffer
	var bufVerbose bytes.Buffer

	buf.WriteString(b.Name)
	bufVerbose.WriteString(b.Name)

	buf.WriteString("(")
	bufVerbose.WriteString("(")

	var params = make([]string, len(paramsList))
	var paramVerbose = make([]string, len(paramsList))
	for index, name := range paramsList {
		params[index] = fmt.Sprintf(`${%d:%v}`, index+1, name)
		paramVerbose[index] = name
	}
	buf.WriteString(strings.Join(params, ", "))
	bufVerbose.WriteString(strings.Join(paramVerbose, ", "))
	if b.IsVariadicParam {
		if paramsList != nil {
			buf.WriteString(fmt.Sprintf(`${%d:, %v}`, len(paramsList)+1, b.ParamTable[len(b.ParamTable)-1]))
			buf.WriteString(", " + b.ParamTable[len(b.ParamTable)-1] + "...")
		} else {
			buf.WriteString(`$1`)
		}
	}
	buf.WriteString(")$0")
	bufVerbose.WriteString(")")
	return buf.String(), bufVerbose.String()
}

type MethodFactory func(*Frame, interface{}) interface{}

//var _title = http.CanonicalHeaderKey

func GetStringBuildInMethod() map[string]*buildinMethod {
	return stringBuildinMethod
}

func GetSliceBuildInMethod() map[string]*buildinMethod {
	return arrayBuildinMethod
}

func GetMapBuildInMethod() map[string]*buildinMethod {
	return mapBuildinMethod
}
