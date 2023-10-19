package standard_parser

type Node interface {
}

type FuzzTag struct {
	TypeName  string
	Method    string
	Labels    []string
	isRawData bool
	Data      []Node // 函数参数
}

// TagDefine 自定义tag类型
type TagDefine struct {
	name  string
	start string
	end   string
}

func NewTagDefine(name, start, end string) *TagDefine {
	return &TagDefine{
		name:  name,
		start: start,
		end:   end,
	}
}
