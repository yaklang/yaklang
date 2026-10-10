package scannode

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/yaklang/yaklang/common/node"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

func nodeRefForSession(nodeID string, session node.SessionState) *nodev1.NodeRef {
	return &nodev1.NodeRef{
		NodeId:        strings.TrimSpace(nodeID),
		NodeSessionId: strings.TrimSpace(session.SessionID),
		CompanyId:     strings.TrimSpace(session.CompanyID),
	}
}

func natsSessionConnectionKey(session node.SessionState) string {
	hash := sha256.New()
	for _, value := range []string{
		session.CompanyID,
		session.SessionID,
		session.NATSURL,
		session.NATSCredentials,
		session.NATSCredentialsExpiresAt.UTC().Format(time.RFC3339Nano),
		session.ExpiresAt.UTC().Format(time.RFC3339Nano),
		session.CommandStream,
		session.CommandConsumer,
		session.InboxPrefix,
		session.CommandSubject,
		session.EventSubjectPrefix,
	} {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func connectNATSForSession(session node.SessionState, connectionName string, extraOptions ...nats.Option) (*nats.Conn, error) {
	if err := validateCompanyNATSSession(session, time.Now()); err != nil {
		return nil, err
	}
	natsURL := strings.TrimSpace(session.NATSURL)
	if natsURL == "" {
		return nil, fmt.Errorf("nats url is required")
	}
	options := []nats.Option{nats.Name(connectionName)}
	if strings.TrimSpace(session.CompanyID) != "" {
		if strings.TrimSpace(session.SessionID) == "" {
			return nil, fmt.Errorf("company nats session id is required")
		}
		if session.NATSCredentialsExpiresAt.IsZero() || !session.NATSCredentialsExpiresAt.After(time.Now()) {
			return nil, fmt.Errorf("company nats credentials are missing or expired")
		}
		credentials := []byte(session.NATSCredentials)
		if _, err := nkeys.ParseDecoratedJWT(credentials); err != nil {
			return nil, fmt.Errorf("parse company nats user jwt: %w", err)
		}
		keyPair, err := nkeys.ParseDecoratedUserNKey(credentials)
		if err != nil {
			return nil, fmt.Errorf("parse company nats user seed: %w", err)
		}
		keyPair.Wipe()
		jwtCallback := func() (string, error) {
			if err := validateCompanyNATSSession(session, time.Now()); err != nil {
				return "", err
			}
			return nkeys.ParseDecoratedJWT(credentials)
		}
		signatureCallback := func(nonce []byte) ([]byte, error) {
			if err := validateCompanyNATSSession(session, time.Now()); err != nil {
				return nil, err
			}
			pair, err := nkeys.ParseDecoratedUserNKey(credentials)
			if err != nil {
				return nil, err
			}
			defer pair.Wipe()
			return pair.Sign(nonce)
		}
		options = append(options,
			nats.UserJWT(jwtCallback, signatureCallback),
			nats.CustomInboxPrefix(strings.TrimSpace(session.InboxPrefix)),
			nats.Secure(nil),
		)
	}
	// The CA path is a trusted startup setting, never supplied by a task or bootstrap response.
	if caFile := strings.TrimSpace(os.Getenv("LEGION_NATS_CA")); caFile != "" {
		options = append(options, nats.RootCAs(caFile))
	}
	options = append(options, extraOptions...)
	conn, err := nats.Connect(natsURL, options...)
	if err != nil {
		return nil, err
	}
	if session.CompanyID != "" {
		// Capture this connection: a renewal creates another connection, whose
		// lifetime is independent of the old credential expiry.
		expires := session.NATSCredentialsExpiresAt
		if session.ExpiresAt.Before(expires) {
			expires = session.ExpiresAt
		}
		time.AfterFunc(time.Until(expires), conn.Close)
	}
	return conn, nil
}

func validateCompanyNATSSession(session node.SessionState, now time.Time) error {
	if strings.TrimSpace(session.CompanyID) == "" {
		return nil
	}
	if session.ExpiresAt.IsZero() || !session.ExpiresAt.After(now) {
		return fmt.Errorf("company node session expired")
	}
	if session.NATSCredentialsExpiresAt.IsZero() || !session.NATSCredentialsExpiresAt.After(now) {
		return fmt.Errorf("company nats credentials expired")
	}
	return nil
}

func sessionScopedOutboundSubject(session node.SessionState, subject string) (string, error) {
	subject = trimSubject(subject)
	if subject == "" {
		return "", fmt.Errorf("outbound subject is required")
	}
	if strings.TrimSpace(session.CompanyID) == "" {
		return subject, nil
	}
	sessionID := strings.TrimSpace(session.SessionID)
	if sessionID == "" || strings.ContainsAny(sessionID, ".*>") {
		return "", fmt.Errorf("company node session id is invalid")
	}
	for _, root := range []string{"legion.event", legionHIDSPrefix, legionRealtimePrefix} {
		if subject == root {
			return "", fmt.Errorf("outbound subject %q has no suffix", subject)
		}
		prefix := root + "."
		if !strings.HasPrefix(subject, prefix) {
			continue
		}
		suffix := strings.TrimPrefix(subject, prefix)
		scopedPrefix := "node." + sessionID + "."
		if strings.HasPrefix(suffix, "node.") {
			if !strings.HasPrefix(suffix, scopedPrefix) {
				return "", fmt.Errorf("outbound subject targets another node session")
			}
			return subject, nil
		}
		return root + "." + scopedPrefix + suffix, nil
	}
	return "", fmt.Errorf("company node outbound subject is outside the session namespace")
}
