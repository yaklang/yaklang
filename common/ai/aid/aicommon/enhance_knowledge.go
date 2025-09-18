package aicommon

type EnhanceKnowledge interface {
	GetContent() string
	GetSource() string
	GetScore() float64
}

type BasicEnhanceKnowledge struct {
	Content string  // 内容
	Source  string  // 来源
	Score   float64 // 相关性评分，0~1之间
}

func NewBasicEnhanceKnowledge(content, source string, score float64) *BasicEnhanceKnowledge {
	return &BasicEnhanceKnowledge{
		Content: content,
		Source:  source,
		Score:   score,
	}
}

func (e *BasicEnhanceKnowledge) GetContent() string {
	if e == nil {
		return ""
	}
	return e.Content
}

func (e *BasicEnhanceKnowledge) GetSource() string {
	if e == nil {
		return ""
	}
	return e.Source
}

func (e *BasicEnhanceKnowledge) GetScore() float64 {
	if e == nil {
		return 0
	}
	return e.Score
}
