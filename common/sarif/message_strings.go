package sarif

type MessageStrings map[string]*MultiformatMessageString

func NewMessageStrings() *MessageStrings {
	return &MessageStrings{}
}

func (m *MessageStrings) WithMessageString(key string, messageString *MultiformatMessageString) *MessageStrings {
	(*m)[key] = messageString
	return m
}
