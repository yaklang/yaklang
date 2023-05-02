package javaclassparser

/*
*
字段/方法
*/
type MemberInfo struct {
	Type               string
	AccessFlags        uint16
	AccessFlagsVerbose []string
	NameIndex          uint16
	NameIndexVerbose   string
	//描述符
	DescriptorIndex        uint16
	DescriptorIndexVerbose string
	//属性表
	Attributes []AttributeInfo
}
