package scannode

import (
	"github.com/yaklang/yaklang/common/node"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
	"time"
)

func TestCompanyCommandRequiresBoundMetadataAcrossFamilies(t *testing.T) {
	now := time.Now()
	session := node.SessionState{CompanyID: "company-a", SessionStartedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), NATSCredentialsExpiresAt: now.Add(time.Hour)}
	for _, suffix := range []string{legionCommandDispatch, legionCommandCancel, legionCommandCapabilityApply, legionCommandAIRuntimeContainerStart, legionCommandSSADebugQuery, legionCommandSSALogTail, legionCommandSSAIRProgramDelete} {
		t.Run(suffix, func(t *testing.T) {
			field := protowire.Number(1)
			if suffix == legionCommandSSADebugQuery || suffix == legionCommandSSALogTail || suffix == legionCommandSSAIRProgramDelete {
				field = 10
			}
			payload := func(company string) []byte {
				raw, _ := proto.Marshal(&nodev1.CommandMetadata{CompanyId: company, IssuedAt: timestamppb.New(now), ExpireAt: timestamppb.New(now.Add(time.Minute))})
				return protowire.AppendBytes(protowire.AppendTag(nil, field, protowire.BytesType), raw)
			}
			subject := "legion.command.node.node-a." + suffix
			if err := validateCompanyCommand(session, subject, payload("company-a"), now); err != nil {
				t.Fatal(err)
			}
			for _, data := range [][]byte{nil, payload(""), payload("company-b"), append(payload("company-a"), payload("company-b")...)} {
				if err := validateCompanyCommand(session, subject, data, now); err == nil {
					t.Fatal("accepted missing or conflicting company metadata")
				}
			}
			if err := validateCompanyCommand(session, subject, payload("company-a"), now.Add(2*time.Hour)); err == nil {
				t.Fatal("accepted expired session")
			}
		})
	}
}

func TestResilienceCompanyCommandRejectsOldGenerationAndExpiredEnvelope(t *testing.T) {
	now := time.Date(2026, 10, 8, 8, 0, 0, 123456789, time.UTC)
	session := node.SessionState{CompanyID: "company-a", SessionStartedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), NATSCredentialsExpiresAt: now.Add(time.Hour)}
	for _, suffix := range []string{legionCommandDispatch, legionCommandSSADebugQuery} {
		t.Run(suffix, func(t *testing.T) {
			for _, tc := range []struct {
				name            string
				issued, expires *timestamppb.Timestamp
				started         time.Time
				allowed         bool
			}{
				{"fresh", timestamppb.New(now), timestamppb.New(now.Add(time.Minute)), session.SessionStartedAt, true},
				{"exact-boundary", timestamppb.New(session.SessionStartedAt), timestamppb.New(now.Add(time.Minute)), session.SessionStartedAt, true},
				{"old-generation-unexpired", timestamppb.New(session.SessionStartedAt.Add(-time.Nanosecond)), timestamppb.New(now.Add(time.Hour)), session.SessionStartedAt, false},
				{"missing-issued", nil, timestamppb.New(now.Add(time.Minute)), session.SessionStartedAt, false},
				{"missing-expiry", timestamppb.New(now), nil, session.SessionStartedAt, false},
				{"invalid-issued", &timestamppb.Timestamp{Nanos: -1}, timestamppb.New(now.Add(time.Minute)), session.SessionStartedAt, false},
				{"invalid-expiry", timestamppb.New(now), &timestamppb.Timestamp{Nanos: -1}, session.SessionStartedAt, false},
				{"expired", timestamppb.New(now.Add(-time.Second)), timestamppb.New(now), session.SessionStartedAt, false},
				{"future-issued", timestamppb.New(now.Add(31 * time.Second)), timestamppb.New(now.Add(time.Minute)), session.SessionStartedAt, false},
				{"invalid-lifetime", timestamppb.New(now.Add(time.Second)), timestamppb.New(now.Add(time.Second)), session.SessionStartedAt, false},
				{"missing-boundary", timestamppb.New(now), timestamppb.New(now.Add(time.Minute)), time.Time{}, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					current := session
					current.SessionStartedAt = tc.started
					raw, err := proto.Marshal(&nodev1.CommandMetadata{CompanyId: session.CompanyID, IssuedAt: tc.issued, ExpireAt: tc.expires})
					if err != nil {
						t.Fatal(err)
					}
					field := protowire.Number(1)
					if suffix == legionCommandSSADebugQuery {
						field = 10
					}
					payload := protowire.AppendBytes(protowire.AppendTag(nil, field, protowire.BytesType), raw)
					err = validateCompanyCommand(current, "legion.command.node.node-a."+suffix, payload, now)
					if (err == nil) != tc.allowed {
						t.Fatalf("allowed=%v error=%v", tc.allowed, err)
					}
				})
			}
		})
	}
}
