package memory_type

import (
	"bytes"
	"fmt"
	"time"
)

// MemoryEntity 表示一个记忆条目
type MemoryEntity struct {
	Id        string
	CreatedAt time.Time
	// 尽量保留原文，适当增加一点点内容的 Content，不准超过1000字，作为记忆来说可用
	Content string
	Tags    []string // 已有 TAG，

	// 7 dims - C.O.R.E. P.A.C.T. Framework (all normalized to 0.0-1.0)
	C_Score float64 // Connectivity Score 这个记忆与其他记忆如何关联？这是一个一次性事实，几乎与其他事实没有什么关联程度
	O_Score float64 // Origin Score 记忆与信息来源确定性，这个来源从哪里来？到底有多少可信度？
	R_Score float64 // Relevance Score 这个信息对用户的目的有多关键？无关紧要？锦上添花？还是成败在此一举？
	E_Score float64 // Emotion Score 用户在表达这个信息时的情绪如何？越低越消极，消极评分时一般伴随信息源不可信
	P_Score float64 // Preference Score 个人偏好对齐评分，这个行为或者问题是否绑定了用户个人风格，品味？
	A_Score float64 // Actionability Score 可操作性评分，是否可以从学习中改进未来行为？
	T_Score float64 // Temporality Score 时效评分，核心问题：这个记忆应该如何被保留？配合时间搜索

	CorePactVector []float32

	// designed for rag searching
	PotentialQuestions []string
}

// SearchResult 搜索结果
type SearchResult struct {
	Entity *MemoryEntity
	Score  float64
}

// ScoreFilter 评分过滤器，用于按C.O.R.E. P.A.C.T.评分搜索
type ScoreFilter struct {
	C_Min, C_Max float64
	O_Min, O_Max float64
	R_Min, R_Max float64
	E_Min, E_Max float64
	P_Min, P_Max float64
	A_Min, A_Max float64
	T_Min, T_Max float64
}

func (r *MemoryEntity) String() string {
	var buf bytes.Buffer
	buf.WriteString("MemoryEntity{\n")
	buf.WriteString("  ID: " + r.Id + "\n")
	buf.WriteString("  Content: " + r.Content + "\n")
	buf.WriteString("  Tags: ")
	for i, tag := range r.Tags {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(tag)
	}
	buf.WriteString("\n")
	buf.WriteString("  C.O.R.E. P.A.C.T. Scores:\n")
	buf.WriteString(fmt.Sprintf("    C=%.2f, O=%.2f, R=%.2f, E=%.2f, P=%.2f, A=%.2f, T=%.2f\n",
		r.C_Score, r.O_Score, r.R_Score, r.E_Score, r.P_Score, r.A_Score, r.T_Score))
	buf.WriteString("  Potential Questions:\n")
	for _, question := range r.PotentialQuestions {
		buf.WriteString("    - " + question + "\n")
	}
	buf.WriteString("}")
	return buf.String()
}

// SearchMemoryResult 搜索记忆的结果
type SearchMemoryResult struct {
	Memories      []*MemoryEntity `json:"memories"`
	TotalContent  string          `json:"total_content"`
	ContentBytes  int             `json:"content_bytes"`
	SearchSummary string          `json:"search_summary"`
}
