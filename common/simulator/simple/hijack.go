package simple

import (
	"github.com/go-rod/rod"
	"regexp"
	"strings"
	"github.com/yaklang/yaklang/common/utils"
)

type ModifyTarget string

var HeadersModifyTarget ModifyTarget = "headers"
var BodyModifyTarget ModifyTarget = "body"
var BodyReplaceTarget ModifyTarget = "bodyReplace"
var HostModifyTarget ModifyTarget = "host"

type baseModify struct {
	modifyUrl    string
	modifyReg    *regexp.Regexp
	modifyTarget ModifyTarget
	modifyResult interface{}
}

type ResponseModification struct {
	baseModify
}

func (modification *baseModify) Generate() error {
	reg, err := regexp.Compile(modification.modifyUrl)
	if err != nil {
		return utils.Errorf("regexp compile %s error: %s", modification.modifyUrl, err)
	}
	modification.modifyReg = reg
	return nil
}

func (modification *baseModify) GetReg() *regexp.Regexp {
	if modification.modifyReg == nil {
		modification.Generate()
	}
	return modification.modifyReg
}

func (modification *ResponseModification) Modify(response *rod.HijackResponse) error {
	switch modification.modifyTarget {
	case BodyModifyTarget:
		return responseBodyModify(response, modification.modifyResult)
	case HeadersModifyTarget:
		return responseHeadersModify(response, modification.modifyResult)
	case BodyReplaceTarget:
		return responseBodyReplace(response, modification.modifyResult)
	default:
		return utils.Errorf("error response modify target.")
	}
}

func responseBodyModify(response *rod.HijackResponse, result interface{}) error {
	switch result.(type) {
	case string:
		bodyStr := result.(string)
		response.SetBody(bodyStr)
		return nil
	default:
		return utils.Errorf("body type error.")
	}
}

func responseBodyReplace(response *rod.HijackResponse, result interface{}) error {
	originBody := response.Body()
	switch result.(type) {
	case []string:
		bodyList := result.([]string)
		for count := 0; count < len(bodyList)-1; count += 2 {
			originBody = strings.Replace(originBody, bodyList[count], bodyList[count+1], -1)
		}
		response.SetBody(originBody)
		return nil
	default:
		return utils.Errorf("body replace data type error.")
	}
}

func responseHeadersModify(response *rod.HijackResponse, result interface{}) error {
	switch result.(type) {
	case map[string]string:
		strMap := result.(map[string]string)
		strList := make([]string, len(strMap)*2)
		for k, v := range strMap {
			strList = append(strList, k, v)
		}
		response.SetHeader(strList...)
	case []string:
		strList := result.([]string)
		response.SetHeader(strList...)
	default:
		return utils.Errorf("headers type error.")
	}
	return nil
}

type RequestModification struct {
	baseModify
}

func (modification *RequestModification) Modify(request *rod.HijackRequest) error {
	switch modification.modifyTarget {
	case HeadersModifyTarget:
		return requestHeadersModify(request, modification.modifyResult)
	case HostModifyTarget:
		return requestHostModify(request, modification.modifyResult)
	case BodyModifyTarget:
		return requestBodyModify(request, modification.modifyResult)
	default:
		return utils.Errorf("error request modify target.")
	}
}

func requestHeadersModify(request *rod.HijackRequest, result interface{}) error {
	switch result.(type) {
	case []string:
		headersList := result.([]string)
		for count := 0; count < len(headersList)-1; count += 2 {
			request.Req().Header.Add(headersList[count], headersList[count+1])
		}
	case map[string]string:
		headersMap := result.(map[string]string)
		for k, v := range headersMap {
			request.Req().Header.Set(k, v)
		}
	default:
		return utils.Errorf("request headers data type error")
	}
	return nil
}

func requestHostModify(request *rod.HijackRequest, result interface{}) error {
	switch result.(type) {
	case string:
		host := result.(string)
		request.Req().Host = host
	default:
		return utils.Errorf("request host data type error")
	}
	return nil
}

func requestBodyModify(request *rod.HijackRequest, result interface{}) error {
	request.SetBody(result)
	return nil
}
