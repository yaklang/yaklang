package authhack

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/dgrijalva/jwt-go"
	"github.com/yaklang/yaklang/common/utils"
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

// JwtParse 解析 JWT 字符串，提取头部与载荷；当提供候选密钥时会尝试逐个验证签名
// 在 yak 中通过 jwt.Parse 调用。不传密钥时仅做解析展示；传入正确密钥时返回该密钥
// 参数:
//   - tokenStr: 待解析的 JWT 字符串
//   - keys: 可选的候选签名密钥列表，用于尝试验证 token 签名
//
// 返回值:
//   - 解析得到的 Token 对象，包含头部与载荷
//   - 验证成功时命中的密钥(字节数组)，未命中时为 nil
//   - 错误信息，解析或验证失败时非 nil
//
// Example:
// ```
// // 先用密钥签发 token，再用同一密钥解析校验，验证往返一致
// token = jwt.JWTGenerate(jwt.ALG_HS256, {"user": "admin"}, []byte("secret123"))~
// tokenObj, key, err = jwt.Parse(token, "secret123")
// assert err == nil, "valid token should parse without error"
// println(string(key))   // OUT: secret123
// assert string(key) == "secret123", "parse should recover the signing key"
// ```
func JwtParse(tokenStr string, keys ...string) (*Token, []byte, error) {
	var jwtErr *jwt.ValidationError
	claims := NewOMapClaims()

	rawToken, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		return nil, nil
	})
	token := NewTokenFromJwtToken(rawToken)
	if err != nil {
		if !errors.As(err, &jwtErr) {
			return nil, nil, utils.Wrap(err, "Unexpected error")
		}
		if jwtErr.Errors == jwt.ValidationErrorMalformed {
			return nil, nil, utils.Errorf("malformed token: %v", err)
		} else if jwtErr.Errors == jwt.ValidationErrorUnverifiable {
			if token != nil && token.Header.Len() > 0 {
				alg := strings.ToLower(fmt.Sprint(token.Header.GetExact("alg")))
				if alg == "none" || alg == "<nil>" {
					// Parse-only (no keys): return token for display. With keys: reject alg:none for verification.
					if len(keys) > 0 {
						return token, nil, ErrAlgNoneNotAllowed
					}
					return token, nil, nil
				}
			}
			return token, nil, utils.Errorf("unverifiable token: %v", err)
		}
	}

	for _, i := range keys {
		key := []byte(i)
		rawToken, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(i), nil
		})
		token := NewTokenFromJwtToken(rawToken)
		if err != nil {
			if !errors.As(err, &jwtErr) {
				return nil, nil, utils.Wrap(err, "Unexpected error")
			}
			if jwtErr.Errors == jwt.ValidationErrorMalformed {
				return nil, nil, utils.Errorf("malformed token: %v", err)
			} else if jwtErr.Errors == jwt.ValidationErrorUnverifiable {
				return token, nil, utils.Errorf("unverifiable token: %v", err)
			} else if jwtErr.Errors&jwt.ValidationErrorSignatureInvalid == jwt.ValidationErrorSignatureInvalid && jwtErr.Inner != nil {
				continue
			} else if jwtErr.Errors&jwt.ValidationErrorAudience == jwt.ValidationErrorAudience {
				return token, key, utils.Errorf("token AUD validation failed: %v", jwtErr.Inner)
			} else if jwtErr.Errors&jwt.ValidationErrorExpired == jwt.ValidationErrorExpired {
				return token, key, utils.Errorf("token EXP validation failed: %v", err)
			} else if jwtErr.Errors&jwt.ValidationErrorIssuedAt == jwt.ValidationErrorIssuedAt {
				return token, key, utils.Errorf("token IAT validation failed: %v", jwtErr.Inner)
			} else if jwtErr.Errors&jwt.ValidationErrorIssuer == jwt.ValidationErrorIssuer {
				return token, key, utils.Errorf("token ISS validation failed: %v", jwtErr.Inner)
			} else if jwtErr.Errors&jwt.ValidationErrorNotValidYet == jwt.ValidationErrorNotValidYet {
				return token, key, utils.Errorf("token NBF validation failed: %v", jwtErr.Inner)
			} else if jwtErr.Errors&jwt.ValidationErrorId == jwt.ValidationErrorId {
				return token, key, utils.Errorf("token JTI validation failed: %v", jwtErr.Inner)
			} else if jwtErr.Errors&jwt.ValidationErrorClaimsInvalid == jwt.ValidationErrorClaimsInvalid {
				return token, nil, utils.Errorf("token claims validation failed: %v", err)
			} else {
				return token, nil, jwtErr
			}
		}

		if token.Header.Len() <= 0 {
			continue
		}

		if !token.Valid {
			continue
		}

		return token, []byte(i), nil
	}

	return token, nil, ErrKeyNotFound
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
