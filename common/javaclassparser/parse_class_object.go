package javaclassparser

import (
	"io/ioutil"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser/attribute_info"
	"github.com/yaklang/yaklang/common/javaclassparser/constant_pool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type ClassObject struct {
	//魔数 class的魔术 -> 0xCAFEBABE
	Type                string
	Magic               uint32
	MinorVersion        uint16
	MajorVersion        uint16
	ConstantPool        []constant_pool.ConstantInfo
	ConstantPoolManager *constant_pool.ConstantPool
	AccessFlags         uint16
	AccessFlagsVerbose  []string
	AccessFlagsToCode   string
	ThisClass           uint16
	ThisClassVerbose    string
	SuperClass          uint16
	SuperClassVerbose   string
	Interfaces          []uint16
	InterfacesVerbose   []string
	Fields              []*MemberInfo
	Methods             []*MemberInfo
	Attributes          []attribute_info.AttributeInfo
}

func NewClassObject() *ClassObject {
	ins := &ClassObject{}
	ins.ConstantPoolManager = constant_pool.NewConstantPoolWithConstant(&ins.ConstantPool)
	return ins
}

//type CLassObjectJson struct {
//	Version      string        `json:"version"`
//	ConstantPool []interface{} `json:"constantPool"`
//	AccessFlags  uint16
//
//	ThisClass      string
//	SuperClass string
//	Interfaces     []string
//	Fields         []*MemberInfo
//	Methods        []*MemberInfo
//	Attributes     []AttributeInfo
//}

//	func (this *ClassObject) MarshalJSON() ([]byte, error) {
//		js := CLassObjectJson{
//			Version: fmt.Sprintf("%d.%d", this.MajorVersion, this.MinorVersion),
//		}
//		return json.MarshalIndent(js, "", " ")
//	}
func (this *ClassObject) Bytes() []byte {
	return _MarshalJavaClass(this, 1)
}
func (this *ClassObject) ToBytesByCustomStringChar(charLength int) []byte {
	return _MarshalJavaClass(this, charLength)
}
func (this *ClassObject) Json() (string, error) {
	s, err := _MarshalToJson(this)
	return string(s), err
}

func (this *ClassObject) Dump() (_ string, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = utils.ErrorStack(e)
		}
	}()
	result, err := NewClassObjectDumper(this).DumpClass()
	if err != nil {
		return "", err
	}
	return result, nil
}

func (this *ClassObject) Bcel() (string, error) {
	bytes := this.Bytes()
	return bytes2bcel(bytes)
}

// 获取
func (this *ClassObject) GetClassName() string {
	name, err := this.getUtf8(this.ThisClass)
	if err != nil {
		return ""
	}
	return name
}
func (this *ClassObject) GetSupperClassName() string {
	name, err := this.getUtf8(this.SuperClass)
	if err != nil {
		return ""
	}
	return name
}
func (this *ClassObject) GetInterfacesName() []string {
	var names []string
	for _, interfaceIndex := range this.Interfaces {
		name, err := this.getUtf8(interfaceIndex)
		if err == nil {
			names = append(names, name)
		}
	}
	return names
}

// 查找
func (this *ClassObject) FindConstStringFromPool(v string) *constant_pool.ConstantUtf8Info {
	n := this.findUtf8IndexFromPool(v)
	if n == -1 {
		return nil
	}
	return this.ConstantPool[n].(*constant_pool.ConstantUtf8Info)
}
func (this *ClassObject) FindFields(v string) *MemberInfo {
	//this.Fields
	return nil
}
func (this *ClassObject) FindMethods(v string) *MemberInfo {
	return nil
}

// SetClassName 修改类名
func (this *ClassObject) SetClassName(name string) error {
	constantInfo, err := this.getConstantInfo(this.ThisClass)
	if err != nil {
		return err
	}

	classInfo, ok := constantInfo.(*constant_pool.ConstantClassInfo)
	if !ok {
		return utils.Errorf("index %d is not ConstantClassInfo", this.ThisClass)
	}
	oldName, ok := this.ConstantPool[classInfo.NameIndex-1].(*constant_pool.ConstantUtf8Info)
	if !ok {
		return utils.Errorf("index %d is not ConstantUtf8Info", this.ThisClass)
	}
	oldName.Value = name
	return nil
}

// SetSourceFileName 设置文件名
func (this *ClassObject) SetSourceFileName(name string) error {
	if !strings.HasSuffix(name, ".java") {
		name = name + ".java"
	}
	var index uint16
	for _, v := range this.Attributes {
		switch v.(type) {
		case *attribute_info.SourceFileAttribute:
			index = v.(*attribute_info.SourceFileAttribute).SourceFileIndex
		}
	}
	oldSourceFileName, ok := this.ConstantPool[index-1].(*constant_pool.ConstantUtf8Info)
	if !ok {
		return utils.Errorf("index %d is not ConstantUtf8Info", index)
	}
	oldSourceFileName.Value = name
	return nil
}

// SetMethodName 设置函数名
func (this *ClassObject) SetMethodName(old, name string) error {
	var index uint16
	for _, v := range this.Methods {
		constantInfo, err := this.getConstantInfo(v.NameIndex)
		if err != nil {
			return err
		}
		utf8Info, ok := constantInfo.(*constant_pool.ConstantUtf8Info)
		if !ok {
			return utils.Errorf("index %d is not ConstantUtf8Info", v.NameIndex)
		}
		if old == utf8Info.Value {
			index = v.NameIndex
		}
	}
	oldMethodName, ok := this.ConstantPool[index-1].(*constant_pool.ConstantUtf8Info)
	if !ok {
		return utils.Errorf("index %d is not ConstantUtf8Info", index)
	}
	oldMethodName.Value = name
	return nil
}

func (this *ClassObject) findUtf8IndexFromPool(v string) int {
	for i := 1; i < len(this.ConstantPool); i++ {
		s, ok := this.ConstantPool[i].(*constant_pool.ConstantUtf8Info)
		if ok {
			if s.Value == v {
				return i
			}
		}
	}
	return -1
}
func (this *ClassObject) getUtf8(index uint16) (string, error) {
	utf8Info, err := this.getConstantInfo(index)
	if err != nil {
		return "", err
	}
	switch ret := utf8Info.(type) {
	case *constant_pool.ConstantIntegerInfo:
	case *constant_pool.ConstantFloatInfo:
	case *constant_pool.ConstantLongInfo:
	case *constant_pool.ConstantDoubleInfo:
	case *constant_pool.ConstantUtf8Info:
		return ret.Value, nil
	case *constant_pool.ConstantStringInfo:
		return this.getUtf8(ret.StringIndex)
	case *constant_pool.ConstantClassInfo:
		return this.getUtf8(ret.NameIndex)
	case *constant_pool.ConstantModuleInfo:
		return this.getUtf8(ret.NameIndex)
	case *constant_pool.ConstantPackageInfo:
		return this.getUtf8(ret.NameIndex)
	case *constant_pool.ConstantFieldrefInfo:
		return this.getUtf8(ret.ClassIndex)
	case *constant_pool.ConstantMethodrefInfo:
		return this.getUtf8(ret.ClassIndex)
	case *constant_pool.ConstantInterfaceMethodrefInfo:
		return this.getUtf8(ret.ClassIndex)
	case *constant_pool.ConstantNameAndTypeInfo:
		return this.getUtf8(ret.NameIndex)
	case *constant_pool.ConstantMethodTypeInfo:
		return this.getUtf8(ret.DescriptorIndex)
	case *constant_pool.ConstantMethodHandleInfo:
	case *constant_pool.ConstantInvokeDynamicInfo:
	}
	return "", utils.Errorf("index %d is not utf8", index)
}
func (this *ClassObject) getConstantInfo(index uint16) (constant_pool.ConstantInfo, error) {
	index -= 1
	if len(this.ConstantPool) <= int(index) {
		return nil, utils.Error("Invalid constant pool index!")
	}
	return this.ConstantPool[index], nil
}
func ParseFromBCEL(data string) (cf *ClassObject, err error) {
	bytes, err := Bcel2bytes(data)
	if err != nil {
		return nil, err
	}
	return Parse(bytes)
}
func ParseFromBase64(base string) (cf *ClassObject, err error) {
	bytes, err := codec.DecodeBase64(base)
	if err != nil {
		return nil, err
	}
	return Parse(bytes)
}
func ParseFromJson(jsonData string) (cf *ClassObject, err error) {
	return _UnmarshalToClassObject(jsonData)
}
func ParseFromFile(path string) (cf *ClassObject, err error) {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(bytes)
}
func Parse(classData []byte) (cf *ClassObject, err error) {
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			var e error
			e, ok = r.(error)
			if !ok {
				e = utils.Errorf("%v", r)
			}
			err = utils.Errorf("parse class error: %v", e)
		}
	}()
	cp := NewClassParser(classData)
	return cp.Parse()
}

func Decompile(i []byte) (string, error) {
	obj, err := Parse(i)
	if err != nil {
		return "", err
	}
	return obj.Dump()
}
