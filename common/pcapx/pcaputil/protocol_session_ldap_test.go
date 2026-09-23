package pcaputil

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProtocolSessionLDAPKerberosKeyAttributeRedaction(t *testing.T) {
	attributes := []string{
		"krbPrincipalKey",
		"krbPrincipalKey;binary",
		"krbExtraData",
		"krbMKey",
		"krbMasterKey",
		"supplementalCredentials",
		"msDS-ManagedPassword",
	}
	for i, attributeName := range attributes {
		t.Run(attributeName, func(t *testing.T) {
			secret := fmt.Sprintf("KERBEROS-KEY-SENTINEL-%d", i)
			attribute := ldapSessionTLV(0x30, append(
				ldapSessionTLV(0x04, []byte(attributeName)),
				ldapSessionTLV(0x31, ldapSessionTLV(0x04, []byte(secret)))...,
			))
			entryBody := append(ldapSessionTLV(0x04, []byte("cn=fixture")), ldapSessionTLV(0x30, attribute)...)
			entry := ldapSessionMsg(0x64, entryBody)

			session, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			t.Cleanup(func() { session.Close("FIN") })
			require.Equal(t, ProbeAccept, session.Probe(entry).Verdict)
			result := session.Feed(0, time.Unix(1, 0), entry)
			require.Nil(t, result.Err, "%v", result.Err)
			require.Len(t, result.Events, 1)

			decoded, err := result.Events[0].Decode()
			require.NoError(t, err)
			encoded, err := json.Marshal(decoded)
			require.NoError(t, err)
			projection := string(encoded)
			require.Contains(t, projection, attributeName, "attribute names remain visible for useful diagnostics")
			require.NotContains(t, projection, secret, "clear key material must not be exported")
			require.NotContains(t, projection, base64.StdEncoding.EncodeToString([]byte(secret)), "base64 key material must not be exported")
			require.Contains(t, projection, `"Redacted":true`)
		})
	}
}

func TestProtocolSessionLDAPSASLStateBudgetChargesRetainedMechanism(t *testing.T) {
	mechanism := strings.Repeat("X", 256)
	tlv := func(tag byte, body []byte) []byte {
		out := []byte{tag}
		switch n := len(body); {
		case n < 128:
			out = append(out, byte(n))
		case n <= 255:
			out = append(out, 0x81, byte(n))
		default:
			out = append(out, 0x82, byte(n>>8), byte(n))
		}
		return append(out, body...)
	}
	sasl := tlv(0xa3, tlv(0x04, []byte(mechanism)))
	body := append([]byte{0x02, 0x01, 0x03}, tlv(0x04, nil)...)
	body = append(body, sasl...)
	op := tlv(0x60, body)
	bind := tlv(0x30, append([]byte{0x02, 0x01, 0x01}, op...))

	var reserved int64
	ldap := &binLDAP{
		pending: make(map[ldapPendingKey]ldapRequest),
		reserveMemory: func(target int64) error {
			reserved = target
			if target > 639 {
				return protocolError(ErrResourceExceeded, "test LDAP capture budget exceeded")
			}
			return nil
		},
	}
	_, err := ldap.consume(0, bind, "LDAPBindRequestFields", 4096)
	require.Error(t, err, "the 256-byte retained SASL mechanism must be charged before insertion")
	require.Equal(t, int64(640), reserved)
	require.Empty(t, ldap.pending, "budget failure must not retain the request")
	require.False(t, ldap.saslBindSucceeded)

	ldap.reserveMemory = func(target int64) error {
		reserved = target
		return nil
	}
	_, err = ldap.consume(0, bind, "LDAPBindRequestFields", 4096)
	require.NoError(t, err)
	require.Equal(t, int64(640), reserved)
	require.Equal(t, mechanism, ldap.pending[ldapPendingKey{direction: 0, messageID: 1}].saslMechanism)
}
