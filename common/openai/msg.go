package openai

type ChatMessage struct {
	Model    string       `json:"model"`
	Messages []ChatDetail `json:"messages"`
}

type ChatDetail struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func NewChatMessage(model string, messages ...ChatDetail) *ChatMessage {
	return &ChatMessage{
		Model:    model,
		Messages: messages,
	}
}

type ChatCompletion struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Choices []ChatChoice `json:"choices"`
	Usage   ChatUsage    `json:"usage"`
}

type ChatChoice struct {
	Index        int        `json:"index"`
	Message      ChatDetail `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
