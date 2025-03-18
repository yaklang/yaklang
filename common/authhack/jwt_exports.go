package authhack

var JWTExports = map[string]interface{}{
	"ALG_NONE":  "None",
	"ALG_ES256": "ES256",
	"ALG_ES384": "ES384",
	"ALG_ES512": "ES512",
	"ALG_HS256": "HS256",
	"ALG_HS384": "HS384",
	"ALG_HS512": "HS512",
	"ALG_RS256": "RS256",
	"ALG_RS384": "RS384",
	"ALG_RS512": "RS512",
	"ALG_PS256": "PS256",
	"ALG_PS384": "PS384",
	"ALG_PS512": "PS512",

	"Parse": JwtParse,
	"JWTGenerate": func(alg string, i any, key []byte) (string, error) {
		return JwtGenerate(alg, i, "JWT", key)
	},
	"JWTGenerateEx": func(alg string, extraHeader, claims any, key []byte) (string, error) {
		return JwtGenerateEx(alg, extraHeader, claims, "JWT", key)
	},
	"JWSGenerate": func(alg string, claims any, key []byte) (string, error) {
		return JwtGenerate(alg, claims, "JWS", key)
	},
	"JWSGenerateEx": func(alg string, extraHeader, claims any, key []byte) (string, error) {
		return JwtGenerateEx(alg, extraHeader, claims, "JWS", key)
	},
	"RemoveAlg":         JwtChangeAlgToNone,
	"AllAlgs":           AvailableJWTTokensAlgs,
	"CommonWeakJWTKeys": WeakJWTTokenKeys,
}
