package sfvm

import (
	"bytes"
	"fmt"
	"github.com/samber/lo"
	"strings"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
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
	UnNameValue      []ValueOperator
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

func (s *SFFrameResult) MergeByResult(result *SFFrameResult) {
	result.SymbolTable.ForEach(func(i string, v ValueOperator) bool {
		if get, b := s.SymbolTable.Get(i); b {
			if merge, err := get.Merge(v); err != nil {
				log.Errorf("merge value fail: %v", err)
				return true
			} else {
				s.SymbolTable.Set(i, merge)
			}
		} else {
			s.SymbolTable.Set(i, v)
		}
		return true
	})
	lo.ForEach(lo.Entries(result.AlertSymbolTable), func(item lo.Entry[string, ValueOperator], index int) {
		s.AlertSymbolTable[item.Key] = item.Value
	})
	lo.ForEach(lo.Entries(result.AlertMsgTable), func(item lo.Entry[string, string], index int) {
		s.AlertMsgTable[item.Key] = item.Value
	})
	lo.ForEach(result.CheckParams, func(item string, index int) {
		s.CheckParams = append(result.CheckParams, item)
	})
	lo.ForEach(result.Errors, func(item string, index int) {
		s.Errors = append(s.Errors, item)
	})
}

type showConfig struct {
	showCode bool
	showDot  bool
	showAll  bool
}
type ShowHandle func(config *showConfig)

func WithShowCode(show bool) ShowHandle {
	return func(config *showConfig) {
		config.showCode = show
	}
}
func WithShowDot(show bool) ShowHandle {
	return func(config *showConfig) {
		config.showDot = show
	}
}
func WithShowAll(show bool) ShowHandle {
	return func(config *showConfig) {
		config.showAll = show
	}
}
func (s *SFFrameResult) Show(opts ...ShowHandle) {
	fmt.Println(s.String(opts...))
}
func (s *SFFrameResult) String(opts ...ShowHandle) string {
	cfg := new(showConfig)
	for _, f := range opts {
		f(cfg)
	}
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
	if s.SymbolTable.Len() > 0 {
		buf.WriteString("Result Vars: \n")
	}
	if cfg.showAll {
		s.SymbolTable.ForEach(func(i string, v ValueOperator) bool {
			showValueMap(buf, i, v, cfg)
			return true
		})
	} else {
		if len(s.AlertSymbolTable) > 0 {
			for name, value := range s.AlertSymbolTable {
				showValueMap(buf, name, value, cfg)
			}
		} else if s.SymbolTable.Len() > 0 {
			s.SymbolTable.ForEach(func(i string, v ValueOperator) bool {
				showValueMap(buf, i, v, cfg)
				return true
			})
		} else {
			// use unName value
			for _, v := range s.UnNameValue {
				showValueMap(buf, "_", v, cfg)
			}
		}
	}
	return buf.String()
}

func showValueMap(buf *bytes.Buffer, varName string, value ValueOperator, cfg *showConfig) {
	var all []ValueOperator
	_ = value.Recursive(func(operator ValueOperator) error {
		all = append(all, operator)
		return nil
	})
	if len(all) == 0 {
		return
	}
	prefixVariable := "  "
	// varName := item.Key
	if !strings.HasPrefix(varName, "$") {
		varName = "$" + varName
	}
	buf.WriteString(prefixVariable + varName + ":\n")
	prefixVariableResult := "    "
	for idxRaw, v := range all {
		var idx = fmt.Sprint(int64(idxRaw + 1))
		if raw, ok := v.(interface{ GetId() int64 }); ok {
			idx = fmt.Sprintf("t%v", raw.GetId())
		}
		buf.WriteString(fmt.Sprintf(prefixVariableResult+"%v: %v\n", idx, utils.ShrinkString(v.String(), 64)))
		rangeIns, ok := v.(interface{ GetRange() memedit.RangeIf })
		if !ok {
			continue
		}
		ssaRange := rangeIns.GetRange()
		if ssaRange == nil {
			continue
		}
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
		buf.WriteString(fmt.Sprintf(
			prefixVariableResult+"    %v:%v:%v - %v:%v\n",
			fileName, start.GetLine(), start.GetColumn(), end.GetLine(), end.GetColumn(),
		))
		if cfg.showCode {
			showValue, ok := v.(interface{ StringWithSourceCode(msg ...string) string })
			if !ok {
				continue
			}
			buf.WriteString(showValue.StringWithSourceCode())
		}

		if cfg.showDot {
			showDot, ok := v.(interface{ DotGraph() string })
			if !ok {
				continue
			}
			buf.WriteString(showDot.DotGraph())
		}
	}
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
