package loop_vuln_verify

import (
	"encoding/json"
	"sync"
)

// VulnContext 待验证的漏洞上下文
type VulnContext struct {
	FilePath        string `json:"file_path"`
	Line            int    `json:"line"`
	SinkFunction    string `json:"sink_function"`
	VulnType        string `json:"vuln_type"`
	Description     string `json:"description"`
	SuspectedSource string `json:"suspected_source,omitempty"`
}

// TraceRecord 数据流追踪记录
type TraceRecord struct {
	Variable string `json:"variable"`
	Location string `json:"location"`
	Source   string `json:"source"`
	Note     string `json:"note"`
}

// FilterRecord 过滤函数记录
type FilterRecord struct {
	Function      string `json:"function"`
	Location      string `json:"location"`
	FilterType    string `json:"filter_type"`   // type_cast, whitelist, blacklist, regex, escape, custom
	Effectiveness string `json:"effectiveness"` // effective, ineffective, uncertain
	Note          string `json:"note"`
}

// Conclusion 验证结论
type Conclusion struct {
	Result           string `json:"result"`     // confirmed, safe, uncertain
	Confidence       string `json:"confidence"` // high, medium, low
	Reason           string `json:"reason"`
	ExploitCondition string `json:"exploit_condition,omitempty"`
	FixSuggestion    string `json:"fix_suggestion,omitempty"`
}

// VerifyState 验证状态管理
type VerifyState struct {
	mu sync.RWMutex

	VulnContext  *VulnContext   `json:"vuln_context,omitempty"`
	ReadFiles    []string       `json:"read_files,omitempty"`
	TraceRecords []TraceRecord  `json:"trace_records,omitempty"`
	Filters      []FilterRecord `json:"filters,omitempty"`
	Conclusion   *Conclusion    `json:"conclusion,omitempty"`
}

// NewVerifyState 创建新的验证状态
func NewVerifyState() *VerifyState {
	return &VerifyState{
		ReadFiles:    make([]string, 0),
		TraceRecords: make([]TraceRecord, 0),
		Filters:      make([]FilterRecord, 0),
	}
}

// SetVulnContext 设置漏洞上下文
func (s *VerifyState) SetVulnContext(ctx *VulnContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.VulnContext = ctx
}

// GetVulnContext 获取漏洞上下文
func (s *VerifyState) GetVulnContext() *VulnContext {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.VulnContext
}

// AddReadFile 添加已读取的文件
func (s *VerifyState) AddReadFile(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 去重
	for _, f := range s.ReadFiles {
		if f == path {
			return
		}
	}
	s.ReadFiles = append(s.ReadFiles, path)
}

// AddTraceRecord 添加追踪记录
func (s *VerifyState) AddTraceRecord(record TraceRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TraceRecords = append(s.TraceRecords, record)
}

// AddFilter 添加过滤函数记录
func (s *VerifyState) AddFilter(filter FilterRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Filters = append(s.Filters, filter)
}

// SetConclusion 设置结论
func (s *VerifyState) SetConclusion(conclusion *Conclusion) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Conclusion = conclusion
}

// GetConclusion 获取结论
func (s *VerifyState) GetConclusion() *Conclusion {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Conclusion
}

// ToMap 转换为 map，用于模板渲染
func (s *VerifyState) ToMap() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := map[string]any{
		"ReadFiles":    s.ReadFiles,
		"TraceRecords": s.TraceRecords,
		"Filters":      s.Filters,
	}

	if s.VulnContext != nil {
		result["VulnContext"] = s.VulnContext
	}
	if s.Conclusion != nil {
		result["Conclusion"] = s.Conclusion
	}

	return result
}

// ToJSON 转换为 JSON 字符串
func (s *VerifyState) ToJSON() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, _ := json.MarshalIndent(s, "", "  ")
	return string(data)
}
