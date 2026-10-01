package aicommon

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// User-data boundaries use a session key separate from projection control tags.
func newUserInputBoundaryKey() string {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		panic("cannot initialize user input boundary key")
	}
	return hex.EncodeToString(secret[:])
}

func userInputBoundaryNonce(key, content string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte("yaklang-user-interact-v1\x00"))
	mac.Write([]byte(content))
	return hex.EncodeToString(mac.Sum(nil))
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
