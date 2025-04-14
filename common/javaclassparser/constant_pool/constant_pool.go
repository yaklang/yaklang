package constant_pool

import "strings"

type ConstantPool struct {
	data *[]ConstantInfo
}

func NewConstantPool() *ConstantPool {
	return &ConstantPool{}
}
func NewConstantPoolWithConstant(data *[]ConstantInfo) *ConstantPool {
	return &ConstantPool{
		data: data,
	}
}

func (c *ConstantPool) GetData() []ConstantInfo {
	return *c.data
}
func (c *ConstantPool) SetData(data []ConstantInfo) {
	*c.data = data
}

func (c *ConstantPool) IndexInfo(index int) ConstantInfo {
	index -= 1
	if index < 0 || index >= len(c.GetData()) {
		return nil
	}
	return c.GetData()[index]
}
func (c *ConstantPool) SearchUtf8Index(s string) int {
	for i, info := range c.GetData() {
		if utf8Info, ok := info.(*ConstantUtf8Info); ok {
			if utf8Info.Value == s {
				return i + 1
			}
		}
	}
	return 0
}
func (c *ConstantPool) SearchUtf8(s string) *ConstantUtf8Info {
	for _, info := range c.GetData() {
		if utf8Info, ok := info.(*ConstantUtf8Info); ok {
			if utf8Info.Value == s {
				return utf8Info
			}
		}
	}
	return nil
}
func (c *ConstantPool) GetUtf8(index int) *ConstantUtf8Info {
	index -= 1
	if index < 0 || index >= len(c.GetData()) {
		return nil
	}
	if utf8Info, ok := c.GetData()[index].(*ConstantUtf8Info); ok {
		return utf8Info
	}
	return nil
}

var ConstantIntegerInfoIns = &ConstantIntegerInfo{}

func (c *ConstantPool) ReplaceConstantInfo(index int, info ConstantInfo) string {
	index -= 1
	if index < 0 || index >= len(c.GetData()) {
		return ""
	}
	c.GetData()[index] = info
	return ""
}
func (c *ConstantPool) AppendConstantInfo(info ConstantInfo) int {
	c.SetData(append(c.GetData(), info))
	return len(c.GetData())
}

func (c *ConstantPool) AddUtf8Info(s string) int {
	index := c.SearchUtf8Index(s)
	if index == 0 {
		nameIns := &ConstantUtf8Info{
			Value: s,
		}
		c.AppendConstantInfo(nameIns)
		index = len(c.GetData())
	}
	return index
}
func (c *ConstantPool) AddNewMethodInfo(className, methodName, desc string) int {
	for i, info1 := range c.GetData() {
		if memberInfo, ok := info1.(*ConstantMethodrefInfo); ok {
			if classInfo := c.GetClassName(int(memberInfo.ClassIndex)); classInfo == className {
				if v, ok := c.IndexInfo(int(memberInfo.NameAndTypeIndex)).(*ConstantNameAndTypeInfo); ok {
					nameStr := c.GetUtf8(int(v.NameIndex))
					descStr := c.GetUtf8(int(v.DescriptorIndex))
					if nameStr != nil && nameStr.Value == methodName && descStr != nil && descStr.Value == desc {
						return i + 1
					}
				}
			}
		}
	}
	memberInfo := &ConstantMethodrefInfo{
		ConstantMemberrefInfo: *c.newMemberrefInfo(className, methodName, desc),
	}
	return c.AppendConstantInfo(memberInfo)
}
func (c *ConstantPool) newMemberrefInfo(className, methodName, desc string) *ConstantMemberrefInfo {
	className = strings.Replace(className, ".", "/", -1)
	// for _, info := range c.GetData() {
	// 	if memberInfo, ok := info.(*ConstantMemberrefInfo); ok {
	// 		if classInfo := c.GetClassName(int(memberInfo.ClassIndex)); classInfo == className {
	// 			if v, ok := c.IndexInfo(int(memberInfo.NameAndTypeIndex)).(*ConstantNameAndTypeInfo); ok {
	// 				nameStr := c.GetUtf8(int(v.NameIndex))
	// 				descStr := c.GetUtf8(int(v.DescriptorIndex))
	// 				if nameStr != nil && nameStr.Value == methodName && descStr != nil && descStr.Value == desc {
	// 					return memberInfo
	// 				}
	// 			}
	// 		}
	// 	}
	// }
	classIndex := c.AddNewClassInfo(className)
	methodNameIndex := c.AddUtf8Info(methodName)
	descIndex := c.AddUtf8Info(desc)
	nameAndType := &ConstantNameAndTypeInfo{
		NameIndex:       uint16(methodNameIndex),
		DescriptorIndex: uint16(descIndex),
	}
	nameAndTypeIndex := c.AppendConstantInfo(nameAndType)
	memberInfo := &ConstantMemberrefInfo{
		ClassIndex:       uint16(classIndex),
		NameAndTypeIndex: uint16(nameAndTypeIndex),
	}
	return memberInfo
}
func (c *ConstantPool) AddNewClassInfo(name string) int {
	name = strings.Replace(name, ".", "/", -1)

	for i, info := range c.GetData() {
		if classInfo, ok := info.(*ConstantClassInfo); ok {
			if utf8Info := c.GetUtf8(int(classInfo.NameIndex)); utf8Info != nil && utf8Info.Value == name {
				return i + 1
			}
		}
	}

	utf8StrIndex := c.AddUtf8Info(name)
	classInfo := &ConstantClassInfo{
		NameIndex: uint16(utf8StrIndex),
	}
	c.AppendConstantInfo(classInfo)
	return len(c.GetData())
}
func (c *ConstantPool) GetClassName(index int) string {
	index -= 1
	if index < 0 || index >= len(c.GetData()) {
		return ""
	}
	if classInfo, ok := c.GetData()[index].(*ConstantClassInfo); ok {
		info := c.GetUtf8(int(classInfo.NameIndex))
		if info == nil {
			return ""
		}
		return info.Value
	}
	return ""
}
