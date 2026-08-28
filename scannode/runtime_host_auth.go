package scannode

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"strings"

	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	"google.golang.org/protobuf/proto"
)

const runtimeHostAuthContext = "ai.runtime.host.v1"

func runtimeHostSessionKey(enrollmentToken, nodeSessionID string) ([]byte, error) {
	token := strings.TrimSpace(enrollmentToken)
	sessionID := strings.TrimSpace(nodeSessionID)
	if token == "" || sessionID == "" {
		return nil, fmt.Errorf("runtime host credential material is incomplete")
	}
	tokenHash := sha256.Sum256([]byte(token))
	mac := hmac.New(sha256.New, tokenHash[:])
	_, _ = mac.Write([]byte(runtimeHostAuthContext))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(sessionID))
	return mac.Sum(nil), nil
}

func verifyRuntimeHostCommand(command *nodev1.AIRuntimeCommand, enrollmentToken, nodeSessionID string) error {
	if command == nil {
		return fmt.Errorf("runtime host command is required")
	}
	key, err := runtimeHostSessionKey(enrollmentToken, nodeSessionID)
	if err != nil {
		return err
	}
	provided := append([]byte(nil), command.AuthTag...)
	command.AuthTag = nil
	payload, marshalErr := (proto.MarshalOptions{Deterministic: true}).Marshal(command)
	command.AuthTag = provided
	if marshalErr != nil {
		return fmt.Errorf("marshal runtime host command: %w", marshalErr)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	if len(provided) == 0 || !hmac.Equal(provided, mac.Sum(nil)) {
		return fmt.Errorf("runtime host command authentication failed")
	}
	return nil
}

func signRuntimeHostResult(result *nodev1.AIRuntimeResult, enrollmentToken, nodeSessionID string) error {
	key, err := runtimeHostSessionKey(enrollmentToken, nodeSessionID)
	if err != nil {
		return err
	}
	result.AuthTag = nil
	payload, err := (proto.MarshalOptions{Deterministic: true}).Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal runtime host result: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	result.AuthTag = mac.Sum(nil)
	return nil
}
