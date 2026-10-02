package stream_parser

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

// RFC 8555 sections 5, 6.2-6.5: passive outer flattened-JWS structure only.
// No signature/key validation, nonce use, resource access or fictitious wire spans.
func inspectACMEJWS(text string) (map[string]any, error) {
	if len(text) > 262144 {
		return nil, fmt.Errorf("acme: JWS exceeds 262144-byte implementation profile")
	}
	object, err := decodeJSONText(text, 32)
	if err != nil || object.Kind != "object" {
		return nil, fmt.Errorf("acme: flattened JWS object required: %v", err)
	}
	if len(object.Keys) != 3 || object.Members["protected"] == nil || object.Members["payload"] == nil || object.Members["signature"] == nil {
		return nil, fmt.Errorf("acme: profile requires exactly protected, payload and signature members")
	}
	decoded := make(map[string][]byte, 3)
	for _, name := range []string{"protected", "payload", "signature"} {
		member := object.Members[name]
		if member.Kind != "string" {
			return nil, fmt.Errorf("acme: %s must be a base64url string", name)
		}
		value, err := acmeBase64URL(member.StringValue, name != "payload")
		if err != nil {
			return nil, fmt.Errorf("acme: %s: %w", name, err)
		}
		decoded[name] = value
	}
	protected, err := decodeJSONText(string(decoded["protected"]), 16)
	if err != nil || protected.Kind != "object" {
		return nil, fmt.Errorf("acme: protected header must decode to a JSON object: %v", err)
	}
	memberString := func(name string) (string, error) {
		v := protected.Members[name]
		if v == nil || v.Kind != "string" || v.StringValue == "" {
			return "", fmt.Errorf("acme: protected %s must be a nonempty string", name)
		}
		return v.StringValue, nil
	}
	alg, err := memberString("alg")
	if err != nil {
		return nil, err
	}
	// Unknown names are not assumed non-MAC or compatible with a key/server.
	switch alg {
	case "ES256", "ES384", "ES512", "RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "EdDSA":
	default:
		return nil, fmt.Errorf("acme: algorithm outside asymmetric implementation profile")
	}
	if protected.Members["crit"] != nil || protected.Members["b64"] != nil {
		return nil, fmt.Errorf("acme: crit/b64 extensions outside encoded-payload profile")
	}
	nonce, err := memberString("nonce")
	if err != nil {
		return nil, err
	}
	nonceBytes, err := acmeBase64URL(nonce, true)
	if err != nil {
		return nil, fmt.Errorf("acme: nonce: %w", err)
	}
	target, err := memberString("url")
	if err != nil {
		return nil, err
	}
	u, err := acmeHTTPSURL(target)
	if err != nil {
		return nil, err
	}
	kid, jwk := protected.Members["kid"], protected.Members["jwk"]
	if (kid == nil) == (jwk == nil) {
		return nil, fmt.Errorf("acme: exactly one of protected kid or jwk is required")
	}
	info := map[string]any{
		"Profile": "ACME outer flattened-JWS structure", "Protected JSON": protected,
		"Protected Bytes": decoded["protected"], "Payload Bytes": decoded["payload"], "Signature Bytes": decoded["signature"],
		"Algorithm": alg, "Nonce": nonce, "Nonce Bytes": nonceBytes, "URL": target, "URL Authority": u.Host, "URL Request Target": u.RequestURI(),
		"POST-as-GET": len(decoded["payload"]) == 0, "Signature Verified": false, "Nonce Freshness Verified": false,
		"Public Key Validated": false, "Account Validated": false, "Server URL Equality Verified": false,
		"TLS Observed": false, "Operation Schema Parsed": false, "Exchange Observed": false,
		"Source Encoding": "base64url", "Decoded Values Are Wire Spans": false,
	}
	if kid != nil {
		if kid.Kind != "string" {
			return nil, fmt.Errorf("acme: kid must be an account URL string")
		}
		if _, err := acmeHTTPSURL(kid.StringValue); err != nil {
			return nil, fmt.Errorf("acme: kid: %w", err)
		}
		info["Key Reference Kind"], info["Account URL"] = "kid", kid.StringValue
	} else {
		// JWK structural members only (RFC 7517/7518/8037). No mathematical
		// key checks, curve support, algorithm compatibility or key use claims.
		if jwk.Kind != "object" || jwk.Members["kty"] == nil || jwk.Members["kty"].Kind != "string" {
			return nil, fmt.Errorf("acme: jwk must be an object with string kty")
		}
		for _, name := range []string{"d", "p", "q", "dp", "dq", "qi", "oth", "k"} {
			if jwk.Members[name] != nil {
				return nil, fmt.Errorf("acme: private or symmetric JWK member outside public-key profile")
			}
		}
		var fields []string
		switch jwk.Members["kty"].StringValue {
		case "RSA":
			fields = []string{"n", "e"}
		case "EC":
			fields = []string{"x", "y"}
		case "OKP":
			fields = []string{"x"}
		default:
			return nil, fmt.Errorf("acme: JWK type outside public-key structure profile")
		}
		if jwk.Members["kty"].StringValue != "RSA" {
			crv := jwk.Members["crv"]
			if crv == nil || crv.Kind != "string" || crv.StringValue == "" {
				return nil, fmt.Errorf("acme: EC/OKP JWK requires nonempty crv")
			}
		}
		for _, name := range fields {
			v := jwk.Members[name]
			if v == nil || v.Kind != "string" {
				return nil, fmt.Errorf("acme: JWK requires string %s", name)
			}
			if _, err := acmeBase64URL(v.StringValue, true); err != nil {
				return nil, fmt.Errorf("acme: JWK %s: %w", name, err)
			}
		}
		info["Key Reference Kind"], info["Public JWK Structure"] = "jwk", jwk
	}
	if len(decoded["payload"]) > 0 {
		payload, err := decodeJSONText(string(decoded["payload"]), 32)
		if err != nil || payload.Kind != "object" {
			return nil, fmt.Errorf("acme: nonempty payload outside JSON-object profile: %v", err)
		}
		info["Payload JSON"] = payload
	}
	return info, nil
}

func acmeBase64URL(value string, nonempty bool) ([]byte, error) {
	if len(value) > 262144 || (nonempty && value == "") || strings.ContainsAny(value, "=\r\n") {
		return nil, fmt.Errorf("invalid unpadded base64url length or alphabet")
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(b) != value {
		return nil, fmt.Errorf("noncanonical unpadded base64url")
	}
	return b, nil
}

func acmeHTTPSURL(value string) (*url.URL, error) {
	info, err := decodeHTTPServiceURL(value)
	if err != nil || info["scheme"] != "https" {
		return nil, fmt.Errorf("acme: absolute HTTPS resource URL required (8192-byte profile): %v", err)
	}
	return url.Parse(value)
}
