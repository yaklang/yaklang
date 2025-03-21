package excelparser

// NodeType 定义节点类型
type NodeType string

const (
	TableNode       NodeType = "table"       // 表格节点
	TextNode        NodeType = "text"        // 文本节点
	URLNode         NodeType = "url"         // URL节点
	FormulaNode     NodeType = "formula"     // 公式节点
	CommentNode     NodeType = "comment"     // 批注节点
	DataConnNode    NodeType = "dataconn"    // 外部数据连接节点
	PowerQueryNode  NodeType = "powerquery"  // Power Query脚本节点
	VBANode         NodeType = "vba"         // VBA宏节点
	HiddenSheetNode NodeType = "hiddensheet" // 隐藏工作表节点
	NameDefNode     NodeType = "namedef"     // 名称管理器定义节点
	CondRuleNode    NodeType = "condrule"    // 条件规则节点
)

// ExcelNode 表示 Excel 文档中的一个节点
type ExcelNode struct {
	Type    NodeType    // 节点类型
	Content interface{} // 节点内容
}

// TableContent 表示表格内容
type TableContent struct {
	SheetName string            // 工作表名称
	Headers   []string          // 表头
	Rows      [][]string        // 数据行
	Metadata  map[string]string // 元数据
}

// TextContent 表示文本内容
type TextContent struct {
	SheetName string // 工作表名称
	Cell      string // 单元格位置
	Text      string // 文本内容
}

// URLContent 表示URL内容
type URLContent struct {
	SheetName string // 工作表名称
	Cell      string // 单元格位置
	URL       string // URL内容
}

// FormulaContent 表示公式内容
type FormulaContent struct {
	SheetName string // 工作表名称
	Cell      string // 单元格位置
	Formula   string // 公式内容
	Result    string // 公式结果
}

// CommentContent 表示批注内容
type CommentContent struct {
	SheetName string // 工作表名称
	Cell      string // 单元格位置
	Author    string // 作者名称
	Text      string // 批注内容
}

// DataConnContent 表示外部数据连接内容
type DataConnContent struct {
	Name             string // 连接名称
	Description      string // 连接描述
	ConnectionString string // 连接字符串
	Command          string // 命令内容
	Type             string // 连接类型
}

// PowerQueryContent 表示Power Query脚本内容
type PowerQueryContent struct {
	Name   string // 查询名称
	Script string // 脚本内容
	Source string // 数据源
}

// VBAContent 表示VBA宏内容
type VBAContent struct {
	ModuleName string // 模块名称
	Code       string // 代码内容
	Type       string // 模块类型
}

// HiddenSheetContent 表示隐藏工作表内容
type HiddenSheetContent struct {
	SheetName string     // 工作表名称
	Headers   []string   // 表头
	Rows      [][]string // 数据行
	HideType  string     // 隐藏类型（普通隐藏、超隐藏）
}

// NameDefContent 表示名称管理器定义内容
type NameDefContent struct {
	Name     string // 名称
	RefersTo string // 引用内容
	Comment  string // 注释
	Scope    string // 作用域
}

// CondRuleContent 表示条件规则内容
type CondRuleContent struct {
	SheetName   string // 工作表名称
	Range       string // 应用区域
	Type        string // 规则类型
	Formula     string // 规则公式
	FormatStyle string // 格式样式
}

// FileType 定义文件类型
type FileType string

const (
	FileTypeText        FileType = "text"        // 文本内容
	FileTypeTable       FileType = "table"       // 表格内容
	FileTypeURL         FileType = "url"         // URL内容
	FileTypeFormula     FileType = "formula"     // 公式内容
	FileTypeComment     FileType = "comment"     // 批注内容
	FileTypeDataConn    FileType = "dataconn"    // 外部数据连接
	FileTypePowerQuery  FileType = "powerquery"  // Power Query脚本
	FileTypeVBA         FileType = "vba"         // VBA宏
	FileTypeHiddenSheet FileType = "hiddensheet" // 隐藏工作表
	FileTypeNameDef     FileType = "namedef"     // 名称管理器定义
	FileTypeCondRule    FileType = "condrule"    // 条件规则
)
