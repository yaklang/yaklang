package standard_parser

import (
	"github.com/yaklang/yaklang/common/utils"
)

type fuzztagPos struct {
	tagType    *TagDefine
	start, end int
	subs       []*fuzztagPos
}

func Parse(code string, tagTypes ...*TagDefine) []Node {
	// 第一层词法: tag
	utagTypes := []*TagDefine{}
	var tagMargins []string
	typeMap := map[string]struct{}{}
	for _, tagType := range tagTypes {
		if _, ok := typeMap[tagType.name]; !ok {
			utagTypes = append(utagTypes, tagType)
			tagMargins = append(tagMargins, tagType.start, tagType.end)
		}
	}

	tagMarginPostions := IndexAllSubstrings(code, tagMargins...)
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
			s := code[pre : pos.start-len(pos.tagType.start)]
			if s != "" {
				res = append(res, StringNode(s))
			}
			res = append(res, tag)
			pre = pos.end + len(pos.tagType.end)
		}
		if pre < end {
			res = append(res, StringNode(code[pre:end]))
		}
		return res
	}
	return newDatasFromPos(0, len(code), filterValidTag(rootTags))
}
