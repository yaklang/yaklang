package aicommon

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// This key is independent of aiprojection's control-tag authority. A visible
// user delimiter must never disclose or grant system/schema/cache authority.
var fallbackUserInputBoundaryKey = newUserInputBoundaryKey()

func newUserInputBoundaryKey() string {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		panic("cannot initialize user input boundary key")
	}
	return hex.EncodeToString(secret[:])
}

func userInputBoundaryNonce(key, content string) string {
	if key == "" {
		key = fallbackUserInputBoundaryKey
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte("yaklang-user-interact-v1\x00"))
	mac.Write([]byte(content))
	return hex.EncodeToString(mac.Sum(nil))
}

// WrapUserInputForPrompt preserves the exact input bytes inside a stable,
// content-bound delimiter. Changing input (even by copying an earlier closing
// delimiter) changes its token. This is a framing aid, not tool authorization
// or a guarantee against natural-language prompt injection.
func (m *Timeline) WrapUserInputForPrompt(content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	key := ""
	if m != nil {
		m.mu.RLock()
		key = m.userInputBoundaryKey
		m.mu.RUnlock()
	}
	return wrapUserInputWithKey(key, content)
}

func wrapUserInputWithKey(key, content string) string {
	nonce := userInputBoundaryNonce(key, content)
	return fmt.Sprintf("<|USER_INTERACT_%s|>\n%s\n<|USER_INTERACT_END_%s|>", nonce, content, nonce)
}

func (m *Timeline) promotableOpenPromptTextLocked(op *PromotableTimelineItem) string {
	if op.Kind == TimelinePromotedKindUserInput && op.Operation == TimelinePromotedOperationUpsert {
		return wrapUserInputWithKey(m.userInputBoundaryKey, op.Payload)
	}
	return op.OpenPromptText()
}
