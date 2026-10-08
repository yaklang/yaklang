package scannode

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/node"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// All command families carry CommandMetadata at field 1. SSA operational
// commands predate that envelope and add it at field 10 without renumbering
// their published identifiers. Check before routing any command handler.
func validateCompanyCommand(session node.SessionState, subject string, payload []byte, now time.Time) error {
	if session.CompanyID == "" {
		return nil
	}
	if err := validateCompanyNATSSession(session, now); err != nil {
		return err
	}
	metadataField := protowire.Number(1)
	for _, suffix := range []string{legionCommandSSADebugQuery, legionCommandSSALogTail, legionCommandSSAIRProgramDelete} {
		if strings.HasSuffix(subject, "."+suffix) {
			metadataField = 10
			break
		}
	}
	found := false
	for len(payload) > 0 {
		number, wireType, size := protowire.ConsumeTag(payload)
		if size < 0 {
			return fmt.Errorf("invalid command protobuf: %w", protowire.ParseError(size))
		}
		payload = payload[size:]
		if number == metadataField {
			if found {
				return fmt.Errorf("duplicate command metadata is not allowed")
			}
			if wireType != protowire.BytesType {
				return fmt.Errorf("command metadata has invalid wire type")
			}
			raw, n := protowire.ConsumeBytes(payload)
			if n < 0 {
				return fmt.Errorf("invalid command metadata: %w", protowire.ParseError(n))
			}
			metadata := &nodev1.CommandMetadata{}
			if err := proto.Unmarshal(raw, metadata); err != nil {
				return fmt.Errorf("invalid command metadata: %w", err)
			}
			if strings.TrimSpace(metadata.CompanyId) != session.CompanyID {
				return fmt.Errorf("command company_id does not match authenticated node session")
			}
			if err := validateCompanyCommandTime(session, metadata, now); err != nil {
				return err
			}
			found = true
			payload = payload[n:]
			continue
		}
		n := protowire.ConsumeFieldValue(number, wireType, payload)
		if n < 0 {
			return fmt.Errorf("invalid command protobuf field: %w", protowire.ParseError(n))
		}
		payload = payload[n:]
	}
	if !found {
		return fmt.Errorf("company command metadata is required")
	}
	return nil
}

// Both timestamps come from the control plane. The session boundary prevents a
// still-unexpired command from an earlier node generation being replayed.
func validateCompanyCommandTime(session node.SessionState, metadata *nodev1.CommandMetadata, now time.Time) error {
	if session.SessionStartedAt.IsZero() || session.SessionStartedAt.After(now.Add(30*time.Second)) {
		return fmt.Errorf("company command requires a valid server session_started_at")
	}
	if metadata.IssuedAt == nil || metadata.IssuedAt.CheckValid() != nil {
		return fmt.Errorf("company command issued_at is missing or invalid")
	}
	if metadata.ExpireAt == nil || metadata.ExpireAt.CheckValid() != nil {
		return fmt.Errorf("company command expire_at is missing or invalid")
	}
	issued, expires := metadata.IssuedAt.AsTime(), metadata.ExpireAt.AsTime()
	if issued.Before(session.SessionStartedAt) {
		return fmt.Errorf("company command predates the authenticated node session")
	}
	if issued.After(now.Add(30 * time.Second)) {
		return fmt.Errorf("company command issued_at is in the future")
	}
	if !expires.After(issued) || !expires.After(now) {
		return fmt.Errorf("company command is expired or has an invalid lifetime")
	}
	return nil
}
