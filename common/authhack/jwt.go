package authhack

import (
	"errors"

	"github.com/dgrijalva/jwt-go"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
)

var (
	ErrKeyNotFound       = utils.Errorf("key not found")
	ErrAlgNoneNotAllowed = utils.Errorf("alg:none token cannot be verified with keys")
	jwtWeakkeyRaw        = `secret
...
012345678901234567890123456789XY
12345
12345678901234567890123456789012
61306132616264382D363136322D343163332D383364362D316366353539623436616663
61306132616264382d363136322d343163332d383364362d316366353539623436616663
872e4e50ce9990d8b041330c47c9ddd11bec6b503ae9386a99da8584e9bb12c4
8zUpiGcaPkNhNGi8oyrq
A43CC200A1BD292682598DA42DAA9FD14589F3D8BF832FFA206BE775259EE1EA
C2A4EB068AF8ABEF18D80B1689C7D785
GQDstcKsx0NHjPOuXOYg5MbeJ1XT0uFiwDVvVBrk
Hello, World!
J5hZTw1vtee0PGaoAuaW
[107 105 97 108 105]
kiali
My super secret key!
Original secret string
R9MyWaEoyiMYViVWo8Fk4TUGWiSoaW6U1nOqXri8_XU
RfxRP43BIKoSQ7P1GfeO
Secret key. You can use ` + "`" + `mix guardian.gen.secret` + "`" + `to get one
SecretKey
Setec Astronomy
SignerTest
Super Secret Key
THE_SAME_HMAC_KEY
ThisIsMySuperSecret
XYZ
YOUR_HMAC_KEY
YoUR sUpEr S3krEt 1337 HMAC kEy HeRE
]V@IaC1%fU,DrVI
` + "`" + `mix guardian.gen.secret` + "`" + `
a43cc200a1bd292682598da42daa9fd14589f3d8bf832ffa206be775259ee1ea
banana
bar
c2a4eb068af8abef18d80b1689c7d785
client_secret_basic
custom
default-key
example-hmac-key
example_key
fe1a1915a379f3be5394b64d14794932
gZH75aKtMN3Yj0iPS4hcgUuTwjAzZr9C
guest
hard!to-guess_secret
has a van
her key
his key
key
key1
key2
key3
kkey
mix guardian.gen.secret
my key
my super secret password
my$ecretK3y
my_very_long_and_safe_secret_key
mypass
mysecretkey
mysupersecretkey
newSecret
password
secret-key
secret123
secret_key
secret_key_here
secretkey
shared-secret
shared_secret
shhhhh
shhhhhhared-secret
some-secret-string
super-secret-password
super_fancy_secret
supersecret
symmetric key
test-key
testing1
token
too many secrets
top secret
verysecret
wrong-secret
xxx
your-256-bit-secret
your-384-bit-secret
your-512-bit-secret
your-own-jwt-secret
your-top-secret-key
jwt
jwt-secret
hmac-secret
hs256-secret
AC8d83&21Almnis710sds
123456`
	JwtAlgs = []jwt.SigningMethod{
		jwt.SigningMethodES384,
		jwt.SigningMethodES256,
		jwt.SigningMethodES512,

		jwt.SigningMethodHS256,
		jwt.SigningMethodHS384,
		jwt.SigningMethodHS512,

		jwt.SigningMethodPS256,
		jwt.SigningMethodPS384,
		jwt.SigningMethodPS512,

		jwt.SigningMethodRS256,
		jwt.SigningMethodRS384,
		jwt.SigningMethodRS512,

		&AuthHackJWTSigningNone{},
	}
	algsToAlgsInstance = map[string]jwt.SigningMethod{}
	WeakJWTTokenKeys   = utils.ParseStringToLines(jwtWeakkeyRaw)
)

func init() {
	for _, i := range JwtAlgs {
		algsToAlgsInstance[i.Alg()] = i
	}
	WeakJWTTokenKeys = utils.RemoveRepeatedWithStringSlice(WeakJWTTokenKeys)
}

func NewJWTHelper(alg string) (*Token, error) {
	if alg == "" || alg == "none" || alg == "None" {
		return NewTokenFromJwtToken(jwt.New(&AuthHackJWTSigningNone{})), nil
	}

	algIns, ok := algsToAlgsInstance[alg]
	if !ok {
		return nil, utils.Errorf("not supported alg: %v in %v", alg, AvailableJWTTokensAlgs())
	}
	return NewTokenFromJwtToken(jwt.New(algIns)), nil
}

// AvailableJWTTokensAlgs 返回当前支持的所有 JWT 签名算法名称列表
// 在 yak 中通过 jwt.AllAlgs 调用
// 返回值:
//   - 支持的签名算法名称字符串切片，如 ES256、HS256、RS256 等
//
// Example:
// ```
// algs = jwt.AllAlgs()
// println(len(algs))   // OUT: 13
// assert len(algs) >= 12, "should expose all supported jwt algorithms"
// ```
func AvailableJWTTokensAlgs() []string {
	var res []string
	for _, i := range JwtAlgs {
		res = append(res, i.Alg())
	}
	return res
}

func JwtGenerate(alg string, claims any, typ string, key []byte) (string, error) {
	return JwtGenerateEx(alg, nil, claims, typ, key)
}

func JwtGenerateEx(alg string, header, claims any, typ string, key []byte) (string, error) {
	token, err := NewJWTHelper(alg)
	if err != nil {
		return "", err
	}
	if header != nil {
		switch h := header.(type) {
		case *orderedmap.OrderedMap:
			// 检查是否包含 alg 和 typ
			hasAlg := h.GetExact("alg") != nil
			hasTyp := h.GetExact("typ") != nil

			if hasAlg && hasTyp {
				// 如果都存在，直接使用传入的 header
				token.Header = h
			} else {
				// 否则遍历设置，保留原有的 alg 和 typ
				h.Range(func(key string, value interface{}) {
					if (key == "alg" && !hasAlg) || (key == "typ" && !hasTyp) {
						return
					}
					token.Header.Set(key, value)
				})
			}
		default:
			headerMap := utils.InterfaceToMapInterface(h)
			// 检查是否包含 alg 和 typ
			_, hasAlg := headerMap["alg"]
			_, hasTyp := headerMap["typ"]

			if hasAlg && hasTyp {
				// 如果都存在，创建新的 OrderedMap
				token.Header = orderedmap.New(headerMap)
			} else {
				// 否则遍历设置，保留原有的 alg 和 typ
				for k, v := range headerMap {
					if (k == "alg" && !hasAlg) || (k == "typ" && !hasTyp) {
						continue
					}
					token.Header.Set(k, v)
				}
			}
		}
	}

	// headers
	if typ == "" {
		token.Header.Set("typ", "JWT")
	} else {
		token.Header.Set("typ", typ)
	}

	// claims
	switch claims := claims.(type) {
	case *orderedmap.OrderedMap:
		token.Claims = NewOMapClaimsFromOrderedMap(claims)
	case *OMapClaims:
		token.Claims = claims
	default:
		newClaims := NewOMapClaims()
		claimMap := utils.InterfaceToMapInterface(claims)
		for k, v := range claimMap {
			newClaims.Set(k, v)
		}
		token.Claims = newClaims
	}

	return token.SignedString(key)
}

// JwtChangeAlgToNone 把给定 JWT 的签名算法改写为 none(去除签名)，常用于 JWT 安全测试
// 在 yak 中通过 jwt.RemoveAlg 调用，保留原始头部与载荷，仅去掉签名部分
// 参数:
//   - token: 原始 JWT 字符串
//
// 返回值:
//   - 算法被改写为 none 的新 JWT 字符串
//   - 错误信息，成功时为 nil
//
// Example:
// ```
// // 把已有 token 改写为 alg:none 形式，验证生成结果可被再次解析出原始 claims
// token = jwt.JWTGenerate(jwt.ALG_HS256, {"user": "admin"}, []byte("secret123"))~
// noneToken = jwt.RemoveAlg(token)~
// tokenObj, _, _ = jwt.Parse(noneToken)
// assert tokenObj != nil, "alg:none token should still be parseable"
// ```
func JwtChangeAlgToNone(token string) (string, error) {
	t, _, err := JwtParse(token)
	if err != nil && !errors.Is(err, ErrKeyNotFound) {
		return "", utils.Errorf("invalid token: %v", token)
	}
	newToken := NewTokenFromJwtToken(jwt.New(&AuthHackJWTSigningNone{}))
	newToken.Header = t.Header
	newToken.Claims = t.Claims
	return newToken.SignedString(nil)
}
