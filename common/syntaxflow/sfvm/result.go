package sfvm

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type SFFrameResult struct {
	// base info
	Rule string
	// additional info
	Description *omap.OrderedMap[string, string]
	CheckParams []string
	Errors      []string
	// value
	SymbolTable      *omap.OrderedMap[string, ValueOperator]
	AlertSymbolTable map[string]ValueOperator
	AlertMsgTable    map[string]string
}

func NewSFResult(rule string) *SFFrameResult {
	return &SFFrameResult{
		Rule:             rule,
		Description:      omap.NewEmptyOrderedMap[string, string](),
		CheckParams:      make([]string, 0),
		SymbolTable:      omap.NewEmptyOrderedMap[string, ValueOperator](),
		AlertSymbolTable: make(map[string]ValueOperator),
		AlertMsgTable:    make(map[string]string),
	}
}

func (s *SFFrameResult) Show() {
	fmt.Println(s.String())
}

func (s *SFFrameResult) String() string {
	buf := bytes.NewBufferString("")
	buf.WriteString(fmt.Sprintf("rule md5 hash: %v\n", codec.Md5(s.Rule)))
	buf.WriteString(fmt.Sprintf("rule preview: %v\n", utils.ShrinkString(s.Rule, 64)))
	buf.WriteString(fmt.Sprintf("description: %v\n", s.Description.String()))
	if len(s.Errors) > 0 {
		buf.WriteString("ERROR:\n")
		prefix := "  "
		for idx, e := range s.Errors {
			buf.WriteString(prefix + fmt.Sprint(idx) + ". " + e + "\n")
		}
		return buf.String()
	}

	count := 0
	if s.SymbolTable.Len() > 0 {
		buf.WriteString("Result Vars: \n")
	}
	if s.SymbolTable.Len() > 1 && s.SymbolTable.Have("_") {
		s.SymbolTable.Delete("_")
		s.SymbolTable.Delete("$_")
	}
	s.SymbolTable.ForEach(func(i string, v ValueOperator) bool {
		count++
		var all []ValueOperator
		_ = v.Recursive(func(operator ValueOperator) error {
			all = append(all, operator)
			return nil
		})
		if len(all) >= 1 {
			prefixVariable := "  "
			varName := i
			if !strings.HasPrefix(varName, "$") {
				varName = "$" + varName
			}
			buf.WriteString(prefixVariable + i + ":\n")
			prefixVariableResult := "    "
			for idxRaw, v := range all {
				var idx = fmt.Sprint(int64(idxRaw + 1))
				if raw, ok := v.(interface{ GetId() int64 }); ok {
					idx = fmt.Sprintf("t%v", raw.GetId())
				}
				buf.WriteString(fmt.Sprintf(prefixVariableResult+"%v: %v\n", idx, utils.ShrinkString(v.String(), 64)))
				if rangeIns, ok := v.(interface{ GetRange() memedit.RangeIf }); ok {
					ssaRange := rangeIns.GetRange()
					if ssaRange != nil {
						start, end := ssaRange.GetStart(), ssaRange.GetEnd()
						editor := ssaRange.GetEditor()
						fileName := editor.GetFilename()
						if fileName == "" {
							var err error
							editor, err = ssadb.GetIrSourceFromHash(editor.SourceCodeMd5())
							if err != nil {
								log.Warn(err)
							}
							if editor != nil {
								fileName = editor.GetFilename()
								if fileName == "" {
									fileName = `[md5:` + editor.SourceCodeMd5() + `]`
								}
							}
						}
						buf.WriteString(fmt.Sprintf(prefixVariableResult+"    %v:%v:%v - %v:%v\n", fileName, start.GetLine(), start.GetColumn(), end.GetLine(), end.GetColumn()))
					}
				}
			}
		}
		return true
	})
	return buf.String()
}

func (s *SFFrameResult) Copy() *SFFrameResult {
	ret := NewSFResult(s.Rule)
	ret.Description = s.Description.Copy()
	ret.CheckParams = append([]string{}, s.CheckParams...)
	ret.Errors = append([]string{}, s.Errors...)
	ret.SymbolTable = s.SymbolTable.Copy()
	ret.AlertSymbolTable = s.AlertSymbolTable
	return ret
}

func (s *SFFrameResult) Name() string {
	for _, name := range []string{
		"title", "name", "desc", "description",
	} {
		result, ok := s.Description.Get(name)
		if !ok {
			continue
		}
		return result
	}
	return utils.ShrinkString(s.String(), 40)
}

func (s *SFFrameResult) GetDescription() string {
	for _, name := range []string{
		"desc", "description", "help",
	} {
		result, ok := s.Description.Get(name)
		if !ok {
			continue
		}
		return result
	}
	return "no description field set"
}
