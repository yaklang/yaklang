package yaklib

import (
	"github.com/yaklang/yaklang/common/gmsm/sm4"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var CodecExports = map[string]interface{}{
	"EncodeToHex":       codec.EncodeToHex,
	"DecodeHex":         codec.DecodeHex,
	"EncodeBase64":      codec.EncodeBase64,
	"DecodeBase64":      codec.DecodeBase64,
	"EncodeBase32":      codec.EncodeBase32,
	"DecodeBase32":      codec.DecodeBase32,
	"EncodeBase64Url":   codec.EncodeBase64Url,
	"DecodeBase64Url":   codec.DecodeBase64Url,
	"Sha1":              codec.Sha1,
	"Sha224":            codec.Sha224,
	"Sha256":            codec.Sha256,
	"Sha384":            codec.Sha384,
	"Sha512":            codec.Sha512,
	"MMH3Hash32":        codec.MMH3Hash32,
	"MMH3Hash128":       codec.MMH3Hash128,
	"MMH3Hash128x64":    codec.MMH3Hash128x64,
	"Md5":               codec.Md5,
	"EncodeUrl":         codec.EncodeUrlCode,
	"DecodeUrl":         codec.QueryUnescape,
	"EscapePathUrl":     codec.PathEscape,
	"UnescapePathUrl":   codec.PathUnescape,
	"EscapeQueryUrl":    codec.QueryEscape,
	"EscapeUrl":         codec.QueryEscape,
	"UnescapeQueryUrl":  codec.QueryUnescape,
	"DoubleEncodeUrl":   codec.DoubleEncodeUrl,
	"DoubleDecodeUrl":   codec.DoubleDecodeUrl,
	"EncodeHtml":        codec.EncodeHtmlEntity,
	"EncodeHtmlHex":     codec.EncodeHtmlEntityHex,
	"EscapeHtml":        codec.EscapeHtmlString,
	"DecodeHtml":        codec.UnescapeHtmlString,
	"EncodeToPrintable": codec.StrConvQuoteHex,
	"EncodeASCII":       codec.StrConvQuoteHex,
	"DecodeASCII":       codec.StrConvUnquote,
	"EncodeChunked":     codec.HTTPChunkedEncode,
	"DecodeChunked":     codec.HTTPChunkedDecode,
	"StrconvQuote":      codec.StrConvQuote,
	"StrconvUnquote":    codec.StrConvUnquote,
	"UTF8ToGBK":         codec.Utf8ToGbk,
	"UTF8ToGB18030":     codec.Utf8ToGB18030,
	"UTF8ToHZGB2312":    codec.Utf8ToHZGB2312,
	"GBKToUTF8":         codec.GbkToUtf8,
	"GB18030ToUTF8":     codec.GB18030ToUtf8,
	"HZGB2312ToUTF8":    codec.HZGB2312ToUtf8,
	"GBKSafe":           codec.GBKSafeString,
	"FixUTF8":           codec.EscapeInvalidUTF8Byte,
	"HTMLChardet":       codec.CharDetect,
	"HTMLChardetBest":   codec.CharDetectBest,

	//
	"PKCS5Padding":         codec.PKCS5Padding,
	"PKCS5UnPadding":       codec.PKCS5UnPadding,
	"PKCS7Padding":         sm4.PKCS7Padding,
	"PKCS7UnPadding":       sm4.PKCS7UnPadding,
	"PKCS7PaddingForDES":   codec.PKCS7PaddingFor8ByteBlock,
	"PKCS7UnPaddingForDES": codec.PKCS7UnPaddingFor8ByteBlock,
	"ZeroPadding":          codec.ZeroPadding,
	"ZeroUnPadding":        codec.ZeroUnPadding,

	// AES
	"AESEncryptBasic": codec.AESEnc,
	"AESDecryptBasic": codec.AESDec,
	"CBC":             codec.CBC,
	"ECB":             codec.ECB,
	"CFB":             codec.CFB,
	"OFB":             codec.OFB,
	"CTR":             codec.CTR,

	// aes
	"AESEncrypt":                    codec.AESEncryptCBCWithPKCSPadding,
	"AESDecrypt":                    codec.AESDecryptCBCWithPKCSPadding,
	"AESCBCEncrypt":                 codec.AESEncryptCBCWithPKCSPadding,
	"AESCBCDecrypt":                 codec.AESDecryptCBCWithPKCSPadding,
	"AESCBCEncryptWithZeroPadding":  codec.AESEncryptCBCWithZeroPadding,
	"AESCBCDecryptWithZeroPadding":  codec.AESDecryptCBCWithZeroPadding,
	"AESCBCEncryptWithPKCS7Padding": codec.AESEncryptCBCWithPKCSPadding,
	"AESCBCDecryptWithPKCS7Padding": codec.AESDecryptCBCWithPKCSPadding,

	"AESECBEncrypt":                 codec.AESEncryptECBWithPKCSPadding,
	"AESECBDecrypt":                 codec.AESDecryptECBWithPKCSPadding,
	"AESECBEncryptWithZeroPadding":  codec.AESEncryptECBWithZeroPadding,
	"AESECBDecryptWithZeroPadding":  codec.AESDecryptECBWithZeroPadding,
	"AESECBEncryptWithPKCS7Padding": codec.AESEncryptECBWithPKCSPadding,
	"AESECBDecryptWithPKCS7Padding": codec.AESDecryptECBWithPKCSPadding,

	"AESGCMEncrypt":                codec.AESGCMEncrypt,
	"AESGCMDecrypt":                codec.AESGCMDecrypt,
	"AESGCMEncryptWithNonceSize16": codec.AESGCMEncryptWithNonceSize16,
	"AESGCMDecryptWithNonceSize16": codec.AESGCMDecryptWithNonceSize16,
	"AESGCMEncryptWithNonceSize12": codec.AESGCMEncryptWithNonceSize12,
	"AESGCMDecryptWithNonceSize12": codec.AESGCMDecryptWithNonceSize12,

	// CFB
	"AESCFBEncrypt":                codec.AESEncryptCFBWithPKCSPadding,
	"AESCFBDecrypt":                codec.AESDecryptCFBWithPKCSPadding,
	"AESEncryptCFBWithPKCSPadding": codec.AESEncryptCFBWithPKCSPadding,
	"AESDecryptCFBWithPKCSPadding": codec.AESDecryptCFBWithPKCSPadding,
	"AESEncryptCFBWithZeroPadding": codec.AESEncryptCFBWithZeroPadding,
	"AESDecryptCFBWithZeroPadding": codec.AESDecryptCFBWithZeroPadding,

	// DES
	"DESEncrypt":    codec.DESEncryptCBCWithZeroPadding,
	"DESDecrypt":    codec.DESDecryptCBCWithZeroPadding,
	"DESCBCEncrypt": codec.DESEncryptCBCWithZeroPadding,
	"DESCBCDecrypt": codec.DESDecryptCBCWithZeroPadding,
	"DESECBEncrypt": codec.DESECBEnc,
	"DESECBDecrypt": codec.DESECBDec,

	"TripleDESEncrypt":    codec.TripleDESEncryptCBCWithZeroPadding,
	"TripleDESDecrypt":    codec.TripleDESDecryptCBCWithZeroPadding,
	"TripleDESCBCEncrypt": codec.TripleDESEncryptCBCWithZeroPadding,
	"TripleDESCBCDecrypt": codec.TripleDESDecryptCBCWithZeroPadding,
	"TripleDESECBEncrypt": codec.TripleDES_ECBEnc,
	"TripleDESECBDecrypt": codec.TripleDES_ECBDec,

	// sm
	"Sm3":                          codec.SM3,
	"Sm4Encrypt":                   codec.SM4EncryptCBCWithPKCSPadding,
	"Sm4Decrypt":                   codec.SM4DecryptCBCWithPKCSPadding,
	"Sm4CBCEncrypt":                codec.SM4EncryptCBCWithPKCSPadding,
	"Sm4CBCDecrypt":                codec.SM4DecryptCBCWithPKCSPadding,
	"Sm4CBCEncryptWithZeroPadding": codec.SM4EncryptCBCWithZeroPadding,
	"Sm4CBCDecryptWithZeroPadding": codec.SM4DecryptCBCWithZeroPadding,
	"Sm4CBCEncryptWithPKCSPadding": codec.SM4EncryptCBCWithPKCSPadding,
	"Sm4CBCDecryptWithPKCSPadding": codec.SM4DecryptCBCWithPKCSPadding,

	"Sm4ECBEncrypt":                codec.SM4EncryptECBWithPKCSPadding,
	"Sm4ECBDecrypt":                codec.SM4DecryptECBWithPKCSPadding,
	"Sm4ECBEncryptWithZeroPadding": codec.SM4EncryptECBWithZeroPadding,
	"Sm4ECBDecryptWithZeroPadding": codec.SM4DecryptECBWithZeroPadding,
	"Sm4ECBEncryptWithPKCSPadding": codec.SM4EncryptECBWithPKCSPadding,
	"Sm4ECBDecryptWithPKCSPadding": codec.SM4DecryptECBWithPKCSPadding,

	"Sm4CFBEncrypt":                codec.SM4EncryptCFBWithPKCSPadding,
	"Sm4CFBDecrypt":                codec.SM4DecryptCFBWithPKCSPadding,
	"Sm4CFBEncryptWithZeroPadding": codec.SM4EncryptCFBWithZeroPadding,
	"Sm4CFBDecryptWithZeroPadding": codec.SM4DecryptCFBWithZeroPadding,
	"Sm4CFBEncryptWithPKCSPadding": codec.SM4EncryptCFBWithPKCSPadding,
	"Sm4CFBDecryptWithPKCSPadding": codec.SM4DecryptCFBWithPKCSPadding,

	"Sm4OFBEncrypt":                codec.SM4EncryptOFBWithPKCSPadding,
	"Sm4OFBDecrypt":                codec.SM4DecryptOFBWithPKCSPadding,
	"Sm4OFBEncryptWithZeroPadding": codec.SM4EncryptOFBWithZeroPadding,
	"Sm4OFBDecryptWithZeroPadding": codec.SM4DecryptOFBWithZeroPadding,
	"Sm4OFBEncryptWithPKCSPadding": codec.SM4EncryptOFBWithPKCSPadding,
	"Sm4OFBDecryptWithPKCSPadding": codec.SM4DecryptOFBWithPKCSPadding,

	"Sm4CTREncrypt":                codec.SM4EncryptCTRWithPKCSPadding,
	"Sm4CTRDecrypt":                codec.SM4DecryptCTRWithPKCSPadding,
	"Sm4CTREncryptWithZeroPadding": codec.SM4EncryptCTRWithZeroPadding,
	"Sm4CTRDecryptWithZeroPadding": codec.SM4DecryptCTRWithZeroPadding,
	"Sm4CTREncryptWithPKCSPadding": codec.SM4EncryptCTRWithPKCSPadding,
	"Sm4CTRDecryptWithPKCSPadding": codec.SM4DecryptCTRWithPKCSPadding,

	"Sm4GCMEncrypt": codec.SM4GCMEncrypt,
	"Sm4GCMDecrypt": codec.SM4GCMDecrypt,

	// 保持向后兼容性的拼写错误函数（已废弃）
	"Sm4EBCEncrypt": codec.SM4EncryptEBCWithPKCSPadding,
	"Sm4EBCDecrypt": codec.SM4DecryptEBCWithPKCSPadding,

	// rc4
	"RC4Encrypt": codec.RC4Encrypt,
	"RC4Decrypt": codec.RC4Decrypt,

	// 智能解码
	"AutoDecode": codec.AutoDecode,

	// HMAC
	"HmacSha1":   codec.HmacSha1,
	"HmacSha256": codec.HmacSha256,
	"HmacSha512": codec.HmacSha512,
	"HmacMD5":    codec.HmacMD5,
	"HmacSM3":    codec.HmacSM3,

	//
	"UnicodeEncode": codec.JsonUnicodeEncode,
	"UnicodeDecode": codec.JsonUnicodeDecode,

	"RSAEncryptWithPKCS1v15": tlsutils.Pkcs1v15Encrypt,
	"RSADecryptWithPKCS1v15": tlsutils.Pkcs1v15Decrypt,
	"RSAEncryptWithOAEP":     tlsutils.PkcsOAEPEncrypt,
	"RSADecryptWithOAEP":     tlsutils.PkcsOAEPDecrypt,

	"Sm2Encrypt":                   codec.SM2EncryptC1C2C3,
	"Sm2Decrypt":                   codec.SM2DecryptC1C2C3,
	"Sm2GenerateHexKeyPair":        codec.GenerateSM2PrivateKeyHEX,
	"Sm2GeneratePemKeyPair":        codec.GenerateSM2PrivateKeyPEM,
	"Sm2EncryptC1C2C3":             codec.SM2EncryptC1C2C3,
	"Sm2DecryptC1C2C3":             codec.SM2DecryptC1C2C3,
	"Sm2DecryptC1C2C3WithPassword": codec.SM2DecryptC1C2C3WithPassword,
	"Sm2EncryptC1C3C2":             codec.SM2EncryptC1C3C2,
	"Sm2DecryptC1C3C2":             codec.SM2DecryptC1C3C2,
	"Sm2DecryptC1C3C2WithPassword": codec.SM2DecryptC1C3C2WithPassword,
	"Sm2EncryptAsn1":               codec.SM2EncryptASN1,
	"Sm2DecryptAsn1WithPassword":   codec.SM2DecryptASN1WithPassword,
	"Sm2DecryptAsn1":               codec.SM2DecryptASN1,

	// SM2 签名和验证函数
	"Sm2SignWithSM3":             codec.SM2SignWithSM3,
	"Sm2SignWithSM3WithPassword": codec.SM2SignWithSM3WithPassword,
	"Sm2VerifyWithSM3":           codec.SM2VerifyWithSM3,

	// SM2 密钥交换函数
	"Sm2KeyExchange":              codec.SM2KeyExchange,
	"Sm2GenerateTemporaryKeyPair": codec.SM2GenerateTemporaryKeyPair,

	"SignSHA256WithRSA":       tlsutils.PemSignSha256WithRSA,
	"SignVerifySHA256WithRSA": tlsutils.PemVerifySignSha256WithRSA,
}
