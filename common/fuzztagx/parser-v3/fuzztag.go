package parser

type Node interface {
}

type FuzzTag struct {
	TypeName string
	Method   string
	Label    string
	Data     []Node // 函数参数
}
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
