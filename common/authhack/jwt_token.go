package authhack

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/dgrijalva/jwt-go"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
)

type Token struct {
	Raw       string                 // The raw token.  Populated when you Parse a token
	Method    jwt.SigningMethod      // The signing method used or to be used
	Header    *orderedmap.OrderedMap // The first segment of the token
	Claims    jwt.Claims             // The second segment of the token
	Signature string                 // The third segment of the token.  Populated when you Parse a token
	Valid     bool                   // Is the token valid?  Populated when you Parse/Verify a token
}

func NewTokenFromJwtToken(old *jwt.Token) *Token {
	token := &Token{
		Raw:       old.Raw,
		Method:    old.Method,
		Header:    orderedmap.New(),
		Claims:    old.Claims,
		Signature: old.Signature,
		Valid:     old.Valid,
	}

	if old.Raw != "" {
		parts := strings.Split(old.Raw, ".")
		// parse Header
		if headerBytes, err := jwt.DecodeSegment(parts[0]); err == nil {
			json.Unmarshal(headerBytes, &token.Header) // ignore error
		}
	} else {
		keys := lo.Keys(old.Header)
		slices.Sort(keys)
		for _, k := range keys {
			token.Header.Set(k, old.Header[k])
		}
	}

	return token
}

// Get the complete, signed token
func (t *Token) SignedString(key interface{}) (string, error) {
	var sig, sstr string
	var err error
	if sstr, err = t.SigningString(); err != nil {
		return "", err
	}
	if sig, err = t.Method.Sign(sstr, key); err != nil {
		return "", err
	}
	return strings.Join([]string{sstr, sig}, "."), nil
}

// Generate the signing string.  This is the
// most expensive part of the whole deal.  Unless you
// need this for something special, just go straight for
// the SignedString.
func (t *Token) SigningString() (string, error) {
	var err error
	parts := make([]string, 2)
	for i, _ := range parts {
		var jsonValue []byte
		if i == 0 {
			if jsonValue, err = json.Marshal(t.Header); err != nil {
				return "", err
			}
		} else {
			if jsonValue, err = json.Marshal(t.Claims); err != nil {
				return "", err
			}
		}

		parts[i] = jwt.EncodeSegment(jsonValue)
	}
	return strings.Join(parts, "."), nil
}
