package standard_parser

import (
	"errors"
	"github.com/yaklang/yaklang/common/utils"
	"math"
)

type fuzztagPos struct {
	tagType    *TagDefine
	start, end int
	subs       []*fuzztagPos
}

func Parse(raw string, tagTypes ...*TagDefine) ([]Node, error) {
	// 用不合法字符表示转义符
	var invilidUnicode rune = 0
	getInvilidUnicode := func() rune {
		invilidUnicode--
		if invilidUnicode == ^math.MaxInt32 {
			return 0
		}
		return invilidUnicode
	}

	// 去重、提取标签边界符
	utagTypes := []*TagDefine{}
	tagBoundaryMap := map[string]rune{} // 转义符映射
	tagBoundaryMapReverse := map[rune]string{}
	var tagBoundarys []string
	typeMap := map[string]struct{}{}
	for _, tagType := range tagTypes {
		if _, ok := typeMap[tagType.name]; !ok {
			l := []string{tagType.start, tagType.end}
			if tagType.start == tagType.end {
				l = []string{tagType.start}
			}
			for _, t := range l {
				if _, ok := tagBoundaryMap[t]; !ok {
					c := getInvilidUnicode()
					if c == 0 {
						return nil, errors.New("too many tags")
					}
					tagBoundaryMapReverse[c] = t
					tagBoundaryMap[t] = c
					tagBoundarys = append(tagBoundarys, t)
				} else {
					return nil, utils.Errorf("tag boundary of tag `%s` conflicts with other tags", tagType.name)
				}
			}
			utagTypes = append(utagTypes, tagType)
		}
	}
	// 执行转义
	escaperMap := map[string]stringx{}
	for k, charx := range tagBoundaryMap {
		escaperMap[k] = []rune{charx}
	}
	escaper := NewEscaper(`\`, escaperMap)
	codex, err := escaper.UnescapeEx(raw)
	if err != nil {
		return nil, err
	}
	tagBoundarysx := []stringx{}
	for _, b := range tagBoundarys {
		tagBoundarysx = append(tagBoundarysx, stringx(b))
	}
	// 查找所有标签位置信息
	tagMarginPostions := IndexAllSubstringsEx(codex, tagBoundarysx...)
	stack := utils.NewStack[*fuzztagPos]()
	rootTags := []*fuzztagPos{}
	for _, pos := range tagMarginPostions {
		tagIndex := pos[0] / 2
		isleft := pos[0]%2 == 0
		typ := tagTypes[tagIndex]
		if isleft {
			tag := &fuzztagPos{start: pos[1] + 2, tagType: typ}
			if stack.Size() != 0 {
				top := stack.Peek()
				top.subs = append(top.subs, tag)
			} else {
				rootTags = append(rootTags, tag)
			}
			stack.Push(tag)
		} else {
			if stack.Size() != 0 {
				if stack.Peek().tagType.name == typ.name {
					top := stack.Pop()
					top.end = pos[1]
				}
			}
		}
	}
	// 过滤未闭合的标签
	var filterValidTag func(rootTags []*fuzztagPos) (result []*fuzztagPos)
	filterValidTag = func(rootTags []*fuzztagPos) (result []*fuzztagPos) {
		for _, tag := range rootTags {
			if tag.end == 0 { // 无效tag
				result = append(result, filterValidTag(tag.subs)...)
			} else {
				result = append(result, tag)
			}
		}
		return
	}
	newStringNode := func(s stringx) StringNode {
		res := ""
		for _, r := range s {
			if v, ok := tagBoundaryMapReverse[r]; ok {
				res += v
			} else {
				res += string(r)
			}
		}
		return StringNode(res)
	}
	// 根据标签位位置信息解析tag
	var newDatasFromPos func(start, end int, poss []*fuzztagPos) []Node
	newDatasFromPos = func(start, end int, poss []*fuzztagPos) []Node {
		pre := start
		res := []Node{}
		for _, pos := range poss {
			if pos.start < start || pos.end > end { // 不解析参数外的fuzztag
				continue
			}
			tag := pos.tagType.NewTag()
			tag.AddData(newDatasFromPos(pos.start, pos.end, pos.subs)...)
			s := codex[pre : pos.start-len(pos.tagType.start)]
			if len(s) != 0 {
				res = append(res, newStringNode(s))
			}
			res = append(res, tag)
			pre = pos.end + len(pos.tagType.end)
		}
		if pre < end {
			res = append(res, newStringNode(codex[pre:end]))
		}
		return res
	}
	return newDatasFromPos(0, len(codex), filterValidTag(rootTags)), nil
}
