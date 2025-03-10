package authhack

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
)

type OMapClaims orderedmap.OrderedMap

func NewOMapClaims() *OMapClaims {
	return &OMapClaims{
		OrderedMapEx: orderedmap.NewOrderMapEx[string, any](nil, nil, false),
	}
}

func NewOMapClaimsFromOrderedMap(m *orderedmap.OrderedMap) *OMapClaims {
	return &OMapClaims{
		OrderedMapEx: m.OrderedMapEx,
	}
}

func (m *OMapClaims) ToOrderedMap() *orderedmap.OrderedMap {
	return &orderedmap.OrderedMap{
		OrderedMapEx: m.OrderedMapEx,
	}
}

func (m *OMapClaims) ToMap() map[string]any {
	return m.ToOrderedMap().ToStringMap()
}

// Compares the aud claim against cmp.
// If required is false, this method will return true if the value matches or is unset
func (m *OMapClaims) VerifyAudience(cmp string, req bool) bool {
	aud, _ := m.GetExact("aud").(string)
	return verifyAud(aud, cmp, req)
}

// Compares the exp claim against cmp.
// If required is false, this method will return true if the value matches or is unset
func (m *OMapClaims) VerifyExpiresAt(cmp int64, req bool) bool {
	switch exp := m.GetExact("exp").(type) {
	case float64:
		return verifyExp(int64(exp), cmp, req)
	case json.Number:
		v, _ := exp.Int64()
		return verifyExp(v, cmp, req)
	}
	return req == false
}

// Compares the iat claim against cmp.
// If required is false, this method will return true if the value matches or is unset
func (m *OMapClaims) VerifyIssuedAt(cmp int64, req bool) bool {
	switch iat := m.GetExact("iat").(type) {
	case float64:
		return verifyIat(int64(iat), cmp, req)
	case json.Number:
		v, _ := iat.Int64()
		return verifyIat(v, cmp, req)
	}
	return req == false
}

// Compares the iss claim against cmp.
// If required is false, this method will return true if the value matches or is unset
func (m *OMapClaims) VerifyIssuer(cmp string, req bool) bool {
	iss, _ := m.GetExact("iss").(string)
	return verifyIss(iss, cmp, req)
}

// Compares the nbf claim against cmp.
// If required is false, this method will return true if the value matches or is unset
func (m *OMapClaims) VerifyNotBefore(cmp int64, req bool) bool {
	switch nbf := m.GetExact("nbf").(type) {
	case float64:
		return verifyNbf(int64(nbf), cmp, req)
	case json.Number:
		v, _ := nbf.Int64()
		return verifyNbf(v, cmp, req)
	}
	return req == false
}

// Validates time based claims "exp, iat, nbf".
// There is no accounting for clock skew.
// As well, if any of the above claims are not in the token, it will still
// be considered a valid claim.
func (m *OMapClaims) Valid() error {
	vErr := new(jwt.ValidationError)
	now := time.Now().Unix()

	if m.VerifyExpiresAt(now, false) == false {
		vErr.Inner = errors.New("Token is expired")
		vErr.Errors |= jwt.ValidationErrorExpired
	}

	if m.VerifyIssuedAt(now, false) == false {
		vErr.Inner = errors.New("Token used before issued")
		vErr.Errors |= jwt.ValidationErrorIssuedAt
	}

	if m.VerifyNotBefore(now, false) == false {
		vErr.Inner = errors.New("Token is not valid yet")
		vErr.Errors |= jwt.ValidationErrorNotValidYet
	}

	if vErr.Errors == 0 {
		return nil
	}

	return vErr
}

func (m *OMapClaims) UnmarshalJSON(data []byte) error {
	if data == nil {
		return nil
	}
	if err := json.Unmarshal(data, (*orderedmap.OrderedMap)(m)); err != nil {
		return err
	}
	return nil
}

func (m *OMapClaims) MarshalJSON() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	if data, err := json.Marshal((*orderedmap.OrderedMap)(m)); err != nil {
		return nil, err
	} else {
		return data, nil
	}
}

// utils
func verifyAud(aud string, cmp string, required bool) bool {
	if aud == "" {
		return !required
	}
	if subtle.ConstantTimeCompare([]byte(aud), []byte(cmp)) != 0 {
		return true
	} else {
		return false
	}
}

func verifyExp(exp int64, now int64, required bool) bool {
	if exp == 0 {
		return !required
	}
	return now <= exp
}

func verifyIat(iat int64, now int64, required bool) bool {
	if iat == 0 {
		return !required
	}
	return now >= iat
}

func verifyIss(iss string, cmp string, required bool) bool {
	if iss == "" {
		return !required
	}
	if subtle.ConstantTimeCompare([]byte(iss), []byte(cmp)) != 0 {
		return true
	} else {
		return false
	}
}

func verifyNbf(nbf int64, now int64, required bool) bool {
	if nbf == 0 {
		return !required
	}
	return now >= nbf
}
