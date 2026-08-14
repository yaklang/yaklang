package aotlib

import "github.com/yaklang/yaklang/common/yak/yaklib/codec"

// CodecExports mirrors the codec module's export table. The implementing
// package is the yaklib/codec subpackage, which does not import the monolithic
// common/yak/yaklib.
var CodecExports = map[string]any{
	"EncodeToHex":     codec.EncodeToHex,
	"DecodeHex":       codec.DecodeHex,
	"EncodeBase64":    codec.EncodeBase64,
	"DecodeBase64":    codec.DecodeBase64,
	"EncodeBase32":    codec.EncodeBase32,
	"DecodeBase32":    codec.DecodeBase32,
	"EncodeBase64Url": codec.EncodeBase64Url,
	"DecodeBase64Url": codec.DecodeBase64Url,
	"Sha1":            codec.Sha1,
	"Sha224":          codec.Sha224,
	"Sha256":          codec.Sha256,
	"Sha384":          codec.Sha384,
	"Sha512":          codec.Sha512,
	"Md5":             codec.Md5,
	"MMH3Hash32":      codec.MMH3Hash32,
	"MMH3Hash128":     codec.MMH3Hash128,
	"EncodeUrl":       codec.EncodeUrlCode,
	"DecodeUrl":       codec.QueryUnescape,
	"EscapePathUrl":   codec.PathEscape,
	"UnescapePathUrl": codec.PathUnescape,
}
