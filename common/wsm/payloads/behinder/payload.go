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
	payloadBytes, err := hex.DecodeString(binPayload)
	if err != nil {
		return nil, err
	}
	code := strings.Replace(string(payloadBytes), "<?", "", 1)
	if v, ok := params["customEncoderFromText"]; ok {
		code += v + "\r\n"
	}
	paramsList := getPhpParams(payloadBytes)
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
	code += "\r\nmain(" + strings.Trim(strings.Join(paramsList, ","), ",") + ");"
	return []byte(code), nil
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
