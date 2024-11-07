package javaclassparser

type ConstantModuleInfo struct {
	Type             string
	NameIndex        uint16
	NameIndexVerbose string
}

func (c ConstantModuleInfo) readInfo(parser *ClassParser) {
	c.NameIndex = parser.reader.readUint16()
}

type ConstantPackageInfo struct {
	Type             string
	NameIndex        uint16
	NameIndexVerbose string
}

func (c ConstantPackageInfo) readInfo(parser *ClassParser) {
	c.NameIndex = parser.reader.readUint16()
}
