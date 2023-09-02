package parser

import (
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

const (
	//TagLeft     = "{{"
	//TagRight    = "}}"
	MethodLeft  = "("
	MethodRight = ")"
	FuzzTagType = "fuzztag"
)

type fuzztagPos struct {
	tagType    *TagDefine
	start, end int
	subs       []*fuzztagPos
}

func isIdentifyString(s string) bool {
	return utils.MatchAllOfRegexp(s, "^[a-zA-Z_][a-zA-Z0-9_:-]*$")
}

func Parse(code string, tagTypes ...*TagDefine) []Node {
	// 第一层词法: tag
	tagTypes = append(tagTypes, NewTagDefine(FuzzTagType, "{{", "}}"))
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
	var newFuzzTagFromPos func(pos *fuzztagPos) (*FuzzTag, bool)
	newFuzzTagFromPos = func(pos *fuzztagPos) (*FuzzTag, bool) {
		tag := &FuzzTag{}
		methodInvokeCode := code[pos.start:pos.end]
		if pos.tagType.name != FuzzTagType {
			tag.Data = []Node{methodInvokeCode}
		}
		matchedPos := IndexAllSubstrings(methodInvokeCode, MethodLeft, MethodRight)
		if len(matchedPos) >= 2 {
			leftPos := matchedPos[0]
			rightPos := matchedPos[len(matchedPos)-1]
			if leftPos[0] == 0 && rightPos[0] == 1 && strings.TrimSpace(methodInvokeCode[rightPos[1]+len(MethodRight):]) == "" {
				methodName := strings.TrimSpace(methodInvokeCode[:leftPos[1]])
				if isIdentifyString(methodName) {
					splits := strings.Split(methodName, "::")
					if len(splits) == 2 {
						tag.Method = splits[0]
						tag.Label = splits[1]
					} else {
						tag.Method = methodName
					}
					if strings.HasPrefix(methodName, "expr:") {
						tag.Data = []Node{code[pos.start+leftPos[1]+1 : pos.start+rightPos[1]]}
					} else {
						tag.Data = newDatasFromPos(pos.start+leftPos[1]+1, pos.start+rightPos[1], pos.subs)
					}
					return tag, true
				}
			}
		} else if len(matchedPos) == 0 {
			if isIdentifyString(methodInvokeCode) {
				tag.Method = methodInvokeCode
				return tag, true
			}
		}
		return nil, false
	}
	newDatasFromPos = func(start, end int, poss []*fuzztagPos) []Node {
		pre := start
		res := []Node{}
		for _, pos := range poss {
			if pos.start < start || pos.end > end { // 不解析参数外的fuzztag
				continue
			}
			tag, ok := newFuzzTagFromPos(pos)
			if ok {
				s := code[pre : pos.start-len(pos.tagType.start)]
				if s != "" {
					res = append(res, s)
				}
				res = append(res, tag)
				pre = pos.end + len(pos.tagType.end)
			}
		}
		if pre < end {
			res = append(res, code[pre:end])
		}
		return res
	}
	return newDatasFromPos(0, len(code), filterValidTag(rootTags))
}
