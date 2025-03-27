package java_decompiler

import (
	"strings"
)

// mockChatAI is a placeholder function that would normally call an AI service
// It combines Java outer class code with inner classes to create a unified view
func (a *Action) mockChatAI(outerClassCode []byte, innerClasses map[string][]byte) ([]byte, error) {
	var result strings.Builder
	result.Write(outerClassCode)
	for _, innerClassCode := range innerClasses {
		result.Write(innerClassCode)
	}

	return []byte(result.String()), nil
}
