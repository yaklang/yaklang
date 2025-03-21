package pptparser

const (
	ExternalNode string = "external"   // 外部节点
	ImageNode    string = "image/png"  // 图像节点
	VideoNode    string = "video/mp4"  // 视频节点
	AudioNode    string = "audio/mpeg" // 音频节点
	TableNode    string = "table"      // 表格节点
	TextNode     string = "text"       // 文本节点
	CommentNode  string = "comment"    // 批注节点
	SlideNode    string = "slide"      // 幻灯片节点
	NoteNode     string = "note"       // 备注节点
)

type SldId struct {
	ID  string
	RID string
}

// PPTNode 表示 PPT 文档中的一个节点
type PPTNode struct {
	Type    string      // 节点类型
	Content interface{} // 节点内容
}

// SlideContent 表示幻灯片内容
type SlideContent struct {
	SlideNumber int               // 幻灯片编号
	Title       string            // 幻灯片标题
	Layout      string            // 幻灯片布局
	Nodes       []PPTNode         //
	Metadata    map[string]string // 元数据
}

// TextContent 表示文本内容
type TextContent struct {
	SlideNumber int               // 幻灯片编号
	Text        string            // 文本内容
	Position    string            // 位置信息
	FontInfo    map[string]string // 字体信息
	IsTitle     bool              // 是否为标题
	Level       int               // 列表级别，0表示不是列表项
}

// ImageContent 表示图像内容
type ImageContent struct {
	SlideNumber int               // 幻灯片编号
	Content     []byte            // 图像内容
	Path        string            // 图像在PPT中的路径
	Alt         string            // 替代文本
	Position    string            // 位置信息
	Size        string            // 尺寸信息
	Metadata    map[string]string // 元数据
}

// TableContent 表示表格内容
type TableContent struct {
	SlideNumber int               // 幻灯片编号
	Headers     []string          // 表头
	Rows        [][]string        // 数据行
	Position    string            // 位置信息
	Style       string            // 表格样式
	Metadata    map[string]string // 元数据
}

// ChartContent 表示图表内容
type ChartContent struct {
	SlideNumber int               // 幻灯片编号
	Type        string            // 图表类型
	Title       string            // 图表标题
	Data        [][]string        // 图表数据
	Position    string            // 位置信息
	Metadata    map[string]string // 元数据
}

// ShapeContent 表示形状内容
type ShapeContent struct {
	SlideNumber int               // 幻灯片编号
	Type        string            // 形状类型
	Text        string            // 形状中的文本
	Position    string            // 位置信息
	Style       string            // 形状样式
	Metadata    map[string]string // 元数据
}

// URLContent 表示URL内容
type URLContent struct {
	SlideNumber int               // 幻灯片编号
	URL         string            // URL内容
	DisplayText string            // 显示文本
	Position    string            // 位置信息
	Metadata    map[string]string // 元数据
}

// NoteContent 表示备注内容
type NoteContent struct {
	SlideNumber int    // 幻灯片编号
	Text        string // 备注内容
}

// CommentContent 表示批注内容
type CommentContent struct {
	SlideNumber int    // 幻灯片编号
	Author      string // 作者名称
	Text        string // 批注内容
	Date        string // 批注日期
	Position    string // 位置信息
}

// MasterSlideContent 表示母版内容
type MasterSlideContent struct {
	Name     string            // 母版名称
	Elements []interface{}     // 母版中的元素
	Metadata map[string]string // 元数据
}

// ThemeContent 表示主题内容
type ThemeContent struct {
	Name       string            // 主题名称
	Colors     []string          // 主题颜色
	Fonts      []string          // 主题字体
	Background string            // 主题背景
	Metadata   map[string]string // 元数据
}

// AnimationContent 表示动画内容
type AnimationContent struct {
	SlideNumber int               // 幻灯片编号
	Sequence    int               // 动画序号
	Effect      string            // 动画效果
	TargetID    string            // 动画目标元素ID
	Duration    string            // 动画持续时间
	Delay       string            // 动画延迟时间
	Metadata    map[string]string // 元数据
}

// TransitionContent 表示转场效果内容
type TransitionContent struct {
	SlideNumber int    // 幻灯片编号
	Type        string // 转场类型
	Duration    string // 转场持续时间
	Sound       string // 转场声音
	Trigger     string // 转场触发方式
}

// VideoContent 表示视频内容
type VideoContent struct {
	SlideNumber int               // 幻灯片编号
	Content     []byte            // 视频内容
	Path        string            // 视频在PPT中的路径
	Position    string            // 位置信息
	Size        string            // 尺寸信息
	Format      string            // 视频格式
	Duration    string            // 视频持续时间
	BinaryData  []byte            // 视频二进制数据
	Metadata    map[string]string // 元数据
}

// AudioContent 表示音频内容
type AudioContent struct {
	SlideNumber int               // 幻灯片编号
	Content     []byte            // 音频内容
	Path        string            // 音频在PPT中的路径
	Format      string            // 音频格式
	Duration    string            // 音频持续时间
	AutoPlay    bool              // 是否自动播放
	Metadata    map[string]string // 元数据
}

// MacroContent 表示宏/VBA内容
type MacroContent struct {
	Name string // 宏名称
	Code string // 宏代码
	Type string // 宏类型
}
