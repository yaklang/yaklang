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

	"Parse":             JwtParse,
	"JWTGenerate":       JWTGenerate,
	"JWTGenerateEx":     JWTGenerateExport,
	"JWSGenerate":       JWSGenerate,
	"JWSGenerateEx":     JWSGenerateExport,
	"RemoveAlg":         JwtChangeAlgToNone,
	"AllAlgs":           AvailableJWTTokensAlgs,
	"CommonWeakJWTKeys": WeakJWTTokenKeys,
}

// JWTGenerate 使用指定签名算法和密钥，把 claims 生成为一个 JWT(typ=JWT) 字符串
// 参数:
//   - alg: 签名算法名称，如 jwt.ALG_HS256、jwt.ALG_NONE 等
//   - i: 载荷 claims，通常是一个 map
//   - key: 签名密钥(字节数组)，HMAC 系列算法直接使用该密钥
//
// 返回值:
//   - 生成的 JWT 字符串
//   - 错误信息，成功时为 nil
//
// Example:
// ```
// // 用 HS256 算法和密钥生成 JWT，再用同一密钥解析校验，验证往返一致
// token = jwt.JWTGenerate(jwt.ALG_HS256, {"user": "admin"}, []byte("secret123"))~
// _, key, err = jwt.Parse(token, "secret123")
// assert err == nil, "valid token should parse without error"
// println(string(key))   // OUT: secret123
// assert string(key) == "secret123", "parse should recover the signing key"
// ```
func JWTGenerate(alg string, i any, key []byte) (string, error) {
	return JwtGenerate(alg, i, "JWT", key)
}

// JWTGenerateExport 在 JWTGenerate 的基础上额外允许自定义 JWT 头部(extraHeader)
// 参数:
//   - alg: 签名算法名称，如 jwt.ALG_HS256
//   - extraHeader: 附加的头部字段，通常是一个 map
//   - claims: 载荷 claims，通常是一个 map
//   - key: 签名密钥(字节数组)
//
// 返回值:
//   - 生成的 JWT 字符串
//   - 错误信息，成功时为 nil
//
// Example:
// ```
// // 生成带自定义头部 kid 的 JWT，并用同一密钥解析校验往返一致
// token = jwt.JWTGenerateEx(jwt.ALG_HS256, {"kid": "k1"}, {"user": "admin"}, []byte("secret123"))~
// _, key, err = jwt.Parse(token, "secret123")
// assert err == nil, "valid token should parse without error"
// println(string(key))   // OUT: secret123
// assert string(key) == "secret123", "parse should recover the signing key"
// ```
func JWTGenerateExport(alg string, extraHeader, claims any, key []byte) (string, error) {
	return JwtGenerateEx(alg, extraHeader, claims, "JWT", key)
}

// JWSGenerate 使用指定签名算法和密钥，把 claims 生成为一个 JWS(typ=JWS) 字符串
// 参数:
//   - alg: 签名算法名称，如 jwt.ALG_HS256
//   - claims: 载荷 claims，通常是一个 map
//   - key: 签名密钥(字节数组)
//
// 返回值:
//   - 生成的 JWS 字符串
//   - 错误信息，成功时为 nil
//
// Example:
// ```
// // 用 HS256 生成 JWS 并解析校验往返一致
// token = jwt.JWSGenerate(jwt.ALG_HS256, {"user": "admin"}, []byte("secret123"))~
// _, key, err = jwt.Parse(token, "secret123")
// assert err == nil, "valid token should parse without error"
// println(string(key))   // OUT: secret123
// assert string(key) == "secret123", "parse should recover the signing key"
// ```
func JWSGenerate(alg string, claims any, key []byte) (string, error) {
	return JwtGenerate(alg, claims, "JWS", key)
}

// JWSGenerateExport 在 JWSGenerate 的基础上额外允许自定义头部(extraHeader)
// 参数:
//   - alg: 签名算法名称，如 jwt.ALG_HS256
//   - extraHeader: 附加的头部字段，通常是一个 map
//   - claims: 载荷 claims，通常是一个 map
//   - key: 签名密钥(字节数组)
//
// 返回值:
//   - 生成的 JWS 字符串
//   - 错误信息，成功时为 nil
//
// Example:
// ```
// // 生成带自定义头部的 JWS 并解析校验往返一致
// token = jwt.JWSGenerateEx(jwt.ALG_HS256, {"kid": "k1"}, {"user": "admin"}, []byte("secret123"))~
// _, key, err = jwt.Parse(token, "secret123")
// assert err == nil, "valid token should parse without error"
// println(string(key))   // OUT: secret123
// assert string(key) == "secret123", "parse should recover the signing key"
// ```
func JWSGenerateExport(alg string, extraHeader, claims any, key []byte) (string, error) {
	return JwtGenerateEx(alg, extraHeader, claims, "JWS", key)
}
