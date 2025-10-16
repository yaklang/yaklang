package sfvm

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type SFFrameResult struct {
	// base info
	config *Config
	rule   *schema.SyntaxFlowRule
	// additional info
	Description *omap.OrderedMap[string, string]
	CheckParams []string
	Errors      []string
	// value
	SymbolTable      *omap.OrderedMap[string, ValueOperator]
	UnNameValue      ValueOperator
	AlertSymbolTable *utils.SafeMap[ValueOperator]
}

func NewSFResult(rule *schema.SyntaxFlowRule, config *Config) *SFFrameResult {
	return &SFFrameResult{
		config:           config,
		rule:             rule,
		Description:      omap.NewEmptyOrderedMap[string, string](),
		CheckParams:      make([]string, 0),
		SymbolTable:      omap.NewEmptyOrderedMap[string, ValueOperator](),
		AlertSymbolTable: utils.NewSafeMap[ValueOperator](),
	}
}

func (s *SFFrameResult) GetAlertInfo(name string) (*schema.SyntaxFlowDescInfo, bool) {
	return s.rule.GetAlertInfo(name)
}
func (s *SFFrameResult) GetRule() *schema.SyntaxFlowRule {
	return s.rule
}

func (s *SFFrameResult) MergeByResult(result *SFFrameResult) {
	result.SymbolTable.ForEach(func(i string, v ValueOperator) bool {
		if value, ok := s.SymbolTable.Get(i); ok {
			if merge, err := value.Merge(v); err != nil {
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
	result.AlertSymbolTable.ForEach(func(key string, value ValueOperator) bool {
		s.AlertSymbolTable.Set(key, value)
		return true
	})
	//for k, v := range result.AlertDesc {
	//	s.AlertDesc[k] = v
	//}
	s.CheckParams = append(s.CheckParams, result.CheckParams...)
	s.Errors = append(s.Errors, result.Errors...)
}

type showConfig struct {
	showCode bool
	showDot  bool
	showAll  bool
}
type ShowOption func(config *showConfig)

func WithShowCode(show ...bool) ShowOption {
	return func(config *showConfig) {
		if len(show) > 0 {
			config.showCode = show[0]
		} else {
			config.showCode = true
		}
	}
}
func WithShowDot(show ...bool) ShowOption {
	return func(config *showConfig) {
		if len(show) > 0 {
			config.showDot = show[0]
		} else {
			config.showDot = true
		}
	}
}
func WithShowAll(show ...bool) ShowOption {
	return func(config *showConfig) {
		if len(show) > 0 {
			config.showAll = show[0]
		} else {
			config.showAll = true
		}
	}
}
func (s *SFFrameResult) Show(opts ...ShowOption) {
	log.Infof(s.String(opts...))
}
func (s *SFFrameResult) String(opts ...ShowOption) string {
	cfg := new(showConfig)
	for _, f := range opts {
		f(cfg)
	}
	buf := bytes.NewBufferString("")
	buf.WriteString(fmt.Sprintf("rule md5 hash: %v\n", codec.Md5(s.rule.Content)))
	buf.WriteString(fmt.Sprintf("rule preview: %v\n", utils.ShrinkString(s.rule.Content, 64)))
	buf.WriteString(fmt.Sprintf("description: %v\n", s.GetDescription()))
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
		if s.AlertSymbolTable.Count() > 0 {
			s.AlertSymbolTable.ForEach(func(key string, value ValueOperator) bool {
				if info, b := s.GetAlertInfo(key); b {
					buf.WriteString(fmt.Sprintf("value: %s description: %v\n", key, codec.AnyToString(info.Msg)))
				}
				showValueMap(buf, key, value, cfg)
				return true
			})
		} else if s.SymbolTable.Len() > 0 {
			s.SymbolTable.ForEach(func(i string, v ValueOperator) bool {
				showValueMap(buf, i, v, cfg)
				return true
			})
		} else {
			// use unName value
			s.UnNameValue.Recursive(func(operator ValueOperator) error {
				showValueMap(buf, "_", operator, cfg)
				return nil
			})
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
		rangeIns, ok := v.(interface{ GetRange() *memedit.Range })
		if !ok {
			continue
		}
		ssaRange := rangeIns.GetRange()
		if ssaRange == nil {
			continue
		}
		start, end := ssaRange.GetStart(), ssaRange.GetEnd()
		editor := ssaRange.GetEditor()
		fileName := editor.GetUrl()
		if fileName == "" {
			var err error
			editor, err = ssadb.GetEditorByHash(editor.GetIrSourceHash())
			if err != nil {
				log.Warn(err)
			}
			if editor != nil {
				fileName = editor.GetFilename()
				if fileName == "" {
					fileName = `[md5:` + editor.GetIrSourceHash() + `]`
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
	ret := NewSFResult(s.rule, s.config)
	ret.Description = s.Description.Copy()
	ret.CheckParams = append([]string{}, s.CheckParams...)
	ret.Errors = append([]string{}, s.Errors...)
	ret.SymbolTable = s.SymbolTable.Copy()
	ret.AlertSymbolTable = s.AlertSymbolTable
	return ret
}

func (s *SFFrameResult) Name() string {
	checkAndHandler := func(str ...string) string {
		for _, s2 := range str {
			if s2 != "" {
				return s2
			}
		}
		return ""
	}
	return checkAndHandler(s.rule.Title, s.rule.TitleZh, s.rule.Description, utils.ShrinkString(s.String(), 40))
}

func (s *SFFrameResult) GetDescription() string {
	if desc := s.rule.Description; desc != "" {
		return desc
	} else {
		info := map[string]string{
			"title":    s.rule.Title,
			"title_zh": s.rule.TitleZh,
			"desc":     s.rule.Description,
			"type":     string(s.rule.Purpose),
			"level":    string(s.rule.Severity),
			"lang":     s.rule.Language,
			"solution": s.rule.Solution,
		}
		return codec.AnyToString(info)
	}
}

func (s *SFFrameResult) ToGRPCModel() *ypb.SyntaxFlowResult {
	if s == nil {
		return nil
	}
	return &ypb.SyntaxFlowResult{
		ResultID:    0,
		TaskID:      "", // TODO: TaskID
		Kind:        "", // TODO: Kind
		ProgramName: "", // TODO: ProgramName

		RuleName:    s.rule.RuleName,
		Title:       s.rule.Title,
		TitleZh:     s.rule.TitleZh,
		Description: s.GetDescription(),
		Severity:    string(s.rule.Severity),
		Purpose:     string(s.rule.Purpose),
		Language:    s.rule.Language,
		RuleContent: s.rule.Content,
	}
}
