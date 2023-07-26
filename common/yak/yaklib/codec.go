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
	"UnescapeQueryUrl":  codec.QueryUnescape,
	"DoubleEncodeUrl":   codec.DoubleEncodeUrl,
	"DoubleDecodeUrl":   codec.DoubleDecodeUrl,
	"EncodeHtml":        codec.EncodeHtmlEntity,
	"EncodeHtmlHex":     codec.EncodeHtmlEntityHex,
	"EscapeHtml":        codec.EscapeHtmlString,
	"DecodeHtml":        codec.UnescapeHtmlString,
	"EncodeToPrintable": codec.StrConvQuote,
	"EncodeASCII":       codec.StrConvQuote,
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
	"PKCS5Padding":   codec.PKCS5Padding,
	"PKCS5UnPadding": codec.PKCS5UnPadding,
	"PKCS7Padding":   sm4.PKCS7Padding,
	"PKCS7UnPadding": sm4.PKCS7UnPadding,
	"ZeroPadding":    codec.ZeroPadding,
	"ZeroUnPadding":  codec.ZeroUnPadding,

	// aes
	"AESEncrypt":                    codec.AESCBCEncrypt,
	"AESDecrypt":                    codec.AESCBCDecrypt,
	"AESCBCEncrypt":                 codec.AESCBCEncrypt,
	"AESCBCDecrypt":                 codec.AESCBCDecrypt,
	"AESCBCEncryptWithZeroPadding":  codec.AESCBCEncryptWithZeroPadding,
	"AESCBCDecryptWithZeroPadding":  codec.AESCBCDecryptWithZeroPadding,
	"AESCBCEncryptWithPKCS7Padding": codec.AESCBCEncryptWithPKCS7Padding,
	"AESCBCDecryptWithPKCS7Padding": codec.AESCBCDecryptWithPKCS7Padding,

	"AESECBEncrypt":                 codec.AESECBEncrypt,
	"AESECBDecrypt":                 codec.AESECBDecrypt,
	"AESECBEncryptWithZeroPadding":  codec.AESECBEncryptWithZeroPadding,
	"AESECBDecryptWithZeroPadding":  codec.AESECBDecryptWithZeroPadding,
	"AESECBEncryptWithPKCS7Padding": codec.AESECBEncryptWithPKCS7Padding,
	"AESECBDecryptWithPKCS7Padding": codec.AESCBCDecryptWithPKCS7Padding,

	"AESGCMEncrypt":                codec.AESGCMEncrypt,
	"AESGCMDecrypt":                codec.AESGCMDecrypt,
	"AESGCMEncryptWithNonceSize16": codec.AESGCMEncryptWithNonceSize16,
	"AESGCMDecryptWithNonceSize16": codec.AESGCMDecryptWithNonceSize16,
	"AESGCMEncryptWithNonceSize12": codec.AESGCMEncryptWithNonceSize12,
	"AESGCMDecryptWithNonceSize12": codec.AESGCMDecryptWithNonceSize12,

	// DES
	"DESEncrypt":    codec.DESCBCEnc,
	"DESDecrypt":    codec.DESCBCDec,
	"DESCBCEncrypt": codec.DESCBCEnc,
	"DESCBCDecrypt": codec.DESCBCDec,
	"DESECBEncrypt": codec.DESECBEnc,
	"DESECBDecrypt": codec.DESECBDec,

	"TripleDESEncrypt":    codec.TripleDES_CBCEnc,
	"TripleDESDecrypt":    codec.TripleDES_CBCDec,
	"TripleDESCBCEncrypt": codec.TripleDES_CBCEnc,
	"TripleDESCBCDecrypt": codec.TripleDES_CBCDec,
	"TripleDESECBEncrypt": codec.TripleDES_ECBEnc,
	"TripleDESECBDecrypt": codec.TripleDES_ECBDec,

	// sm
	"Sm3":           codec.SM3,
	"Sm4CBCEncrypt": codec.SM4CBCEnc,
	"Sm4CBCDecrypt": codec.SM4CBCDec,
	"Sm4CFBEncrypt": codec.SM4CFBEnc,
	"Sm4CFBDecrypt": codec.SM4CFBDec,
	"Sm4ECBEncrypt": codec.SM4ECBEnc,
	"Sm4ECBDecrypt": codec.SM4ECBDec,
	"Sm4EBCEncrypt": codec.SM4ECBEnc,
	"Sm4EBCDecrypt": codec.SM4ECBDec,
	"Sm4OFBEncrypt": codec.SM4OFBEnc,
	"Sm4OFBDecrypt": codec.SM4OFBDec,
	"Sm4GCMEncrypt": codec.SM4GCMEnc,
	"Sm4GCMDecrypt": codec.SM4GCMDec,

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

	"RSAEncryptWithPKCS1v15": tlsutils.PemPkcs1v15Encrypt,
	"RSADecryptWithPKCS1v15": tlsutils.PemPkcs1v15Decrypt,
	"RSAEncryptWithOAEP":     tlsutils.PemPkcsOAEPEncrypt,
	"RSADecryptWithOAEP":     tlsutils.PemPkcs1v15Decrypt,

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
}
