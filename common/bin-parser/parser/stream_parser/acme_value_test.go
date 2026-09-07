package stream_parser

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const acmeUnitProtected = `{"alg":"ES256","nonce":"AQIDBA","url":"https://acme.example/acme/order/1","kid":"https://acme.example/acme/account/1"}`

func acmeUnitJWS(protected, payload string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return fmt.Sprintf(`{"protected":%q,"payload":%q,"signature":%q}`, enc([]byte(protected)), enc([]byte(payload)), enc(make([]byte, 64)))
}

func TestACMEJWSStructures(t *testing.T) {
	for _, payload := range []string{"", `{}`, `{"identifiers":[{"type":"dns","value":"example.test"}]}`} {
		info, err := inspectACMEJWS(acmeUnitJWS(acmeUnitProtected, payload))
		require.NoError(t, err)
		require.Equal(t, "ES256", info["Algorithm"])
		require.Equal(t, []byte{1, 2, 3, 4}, info["Nonce Bytes"])
		require.Equal(t, []byte(payload), info["Payload Bytes"])
		require.Equal(t, make([]byte, 64), info["Signature Bytes"])
		require.Equal(t, payload == "", info["POST-as-GET"])
		require.Equal(t, "/acme/order/1", info["URL Request Target"])
		for _, key := range []string{"Signature Verified", "Nonce Freshness Verified", "Public Key Validated", "Account Validated", "Server URL Equality Verified", "TLS Observed", "Operation Schema Parsed", "Exchange Observed", "Decoded Values Are Wire Spans"} {
			require.Equal(t, false, info[key], key)
		}
	}
	for _, key := range []string{`{"kty":"RSA","n":"AQID","e":"AQAB"}`, `{"kty":"EC","crv":"P-256","x":"AQID","y":"BAUG"}`, `{"kty":"OKP","crv":"Ed25519","x":"AQID"}`} {
		// Structural literals are not mathematically validated usable keys.
		p := strings.Replace(acmeUnitProtected, `"kid":"https://acme.example/acme/account/1"`, `"jwk":`+key, 1)
		info, err := inspectACMEJWS(acmeUnitJWS(p, `{}`))
		require.NoError(t, err)
		require.Equal(t, "jwk", info["Key Reference Kind"])
		require.Equal(t, false, info["Public Key Validated"])
	}
}

func TestACMEResourceURLs(t *testing.T) {
	for _, s := range []string{"https://a/a b", "https://a/?bad=%xy", "https://a:65536/", "https://u:p@a/", "https://a/#", "https://a/路径", "https://a/a\\b", "https://a/a<", "https://[not-ip]/", "https://a:/"} {
		_, err := acmeHTTPSURL(s)
		require.Error(t, err, s)
	}
	for _, s := range []string{"https://a/a%20b", "https://a/?x=1", "https://[2001:db8::1]:443/"} {
		_, err := acmeHTTPSURL(s)
		require.NoError(t, err, s)
	}
}

func TestACMEJWSRejectedStructures(t *testing.T) {
	valid := acmeUnitJWS(acmeUnitProtected, `{}`)
	invalid := []string{`{}`, `[]`, `null`, `{"protected":null,"payload":"","signature":"AA"}`, valid + `{}`, strings.Replace(valid, `"payload":`, `"payload":"","payload":`, 1), strings.Repeat(" ", 262145)}
	for _, p := range []string{
		strings.Replace(acmeUnitProtected, `"ES256"`, `"none"`, 1),
		strings.Replace(acmeUnitProtected, `"ES256"`, `"HS256"`, 1),
		strings.Replace(acmeUnitProtected, `"ES256"`, `"future-alg"`, 1),
		strings.Replace(acmeUnitProtected, `"nonce":"AQIDBA"`, `"nonce":"AQIDBA=="`, 1),
		strings.Replace(acmeUnitProtected, `"nonce":"AQIDBA"`, `"nonce":"AB"`, 1),
		strings.Replace(acmeUnitProtected, `"nonce":"AQIDBA"`, `"nonce":null`, 1),
		strings.Replace(acmeUnitProtected, `"url":"https:`, `"url":"http:`, 1),
		strings.Replace(acmeUnitProtected, `/order/1"`, `/order/1#fragment"`, 1),
		strings.Replace(acmeUnitProtected, `"kid":"https://acme.example/acme/account/1"`, `"jwk":{"kty":"oct","k":"AA"}`, 1),
		strings.Replace(acmeUnitProtected, `"kid":"https://acme.example/acme/account/1"`, `"jwk":{"kty":"EC","crv":"P-256","x":"AA"}`, 1),
		strings.Replace(acmeUnitProtected, `"kid":"https://acme.example/acme/account/1"`, `"other":1`, 1),
		strings.Replace(acmeUnitProtected, `"alg":`, `"jwk":{},"alg":`, 1),
		strings.Replace(acmeUnitProtected, `"alg":`, `"crit":[],"alg":`, 1),
		strings.Replace(acmeUnitProtected, `"alg":`, `"b64":false,"alg":`, 1),
		strings.Replace(acmeUnitProtected, `"alg":`, `"alg":"RS256","alg":`, 1),
	} {
		invalid = append(invalid, acmeUnitJWS(p, `{}`))
	}
	for _, payload := range []string{`null`, `[]`, `{"a":1,"a":2}`, "\xff", `{"a":"\ud800"}`} {
		invalid = append(invalid, acmeUnitJWS(acmeUnitProtected, payload))
	}
	for i, wire := range invalid {
		_, err := inspectACMEJWS(wire)
		require.Error(t, err, "case %d", i)
	}
	for _, s := range []string{"=", "A", "AB", "AA=", "AA\n", "A+", "A/", "AA "} {
		_, err := acmeBase64URL(s, true)
		require.Error(t, err, s)
	}
}
