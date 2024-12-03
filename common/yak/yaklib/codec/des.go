package codec

import (
	"crypto/des"
	"fmt"
)

//func desEnc(key []byte, data []byte, iv []byte, mode func(cipher.Block, []byte) cipher.BlockMode, isTripleDES bool) ([]byte, error) {
//	var block cipher.Block
//	var err error
//	if iv == nil {
//		iv = make([]byte, 8)
//	} else {
//		if len(iv)%8 != 0 {
//			iv = ZeroPadding(iv, 8)
//		}
//	}
//	if len(key)%8 != 0 {
//		key = ZeroPadding(key, 8)
//		if len(key) > 8 {
//			key = key[:8]
//		}
//	}
//
//	if isTripleDES {
//		block, err = des.NewTripleDESCipher(key)
//	} else {
//		block, err = des.NewCipher(key)
//	}
//
//	if err != nil {
//		return nil, errors.Errorf("create cipher failed: %s", err)
//	}
//
//	if len(data)%8 != 0 {
//		data = ZeroPadding(data, 8)
//	}
//
//	cbcMode := mode(block, iv)
//	result := make([]byte, len(data))
//	cbcMode.CryptBlocks(result, data)
//	return result, nil
//}
//
//func desDec(key []byte, data []byte, iv []byte, mode func(cipher.Block, []byte) cipher.BlockMode, isTripleDES bool) ([]byte, error) {
//	var block cipher.Block
//	var err error
//	if iv == nil {
//		iv = make([]byte, 8)
//	} else {
//		if len(iv)%8 != 0 {
//			iv = ZeroPadding(iv, 8)
//		}
//	}
//	if len(key)%8 != 0 {
//		key = ZeroPadding(key, 8)
//		if len(key) > 8 {
//			key = key[:8]
//		}
//	}
//
//	if isTripleDES {
//		block, err = des.NewTripleDESCipher(key)
//	} else {
//		block, err = des.NewCipher(key)
//	}
//
//	if err != nil {
//		return nil, errors.Errorf("create cipher failed: %s", err)
//	}
//
//	if len(data)%8 != 0 {
//		data = ZeroPadding(data, 8)
//	}
//
//	cbcMode := mode(block, iv)
//	result := make([]byte, len(data))
//	cbcMode.CryptBlocks(result, data)
//	return result, nil
//}
//
//func DESCBCEncEx(key []byte, data []byte, iv []byte, isTripleDES bool) ([]byte, error) {
//	return desEnc(key, data, iv, cipher.NewCBCEncrypter, isTripleDES)
//}
//
//func DESCBCDecEx(key, data, iv []byte, isTripleDES bool) ([]byte, error) {
//	return desDec(key, data, iv, cipher.NewCBCDecrypter, isTripleDES)
//}
//
//func DESECBEncEx(key []byte, data []byte, isTripleDES bool) ([]byte, error) {
//	var block cipher.Block
//	var err error
//	blockSize := 8
//
//	if len(key)%blockSize != 0 {
//		key = ZeroPadding(key, blockSize)
//		if len(key) > 8 {
//			key = key[:8]
//		}
//	}
//
//	if isTripleDES {
//		block, err = des.NewTripleDESCipher(key)
//	} else {
//		block, err = des.NewCipher(key)
//	}
//	if err != nil {
//		return nil, fmt.Errorf("DES ECB Error: %s", err)
//	}
//	if len(data)%blockSize != 0 {
//		data = ZeroPadding(data, blockSize)
//	}
//
//	encrypted := make([]byte, len(data))
//	for bs, be := 0, blockSize; bs < len(data); bs, be = bs+blockSize, be+blockSize {
//		block.Encrypt(encrypted[bs:be], data[bs:be])
//	}
//	return encrypted, nil
//}
//
//func DESECBDecEx(key []byte, data []byte, isTripleDES bool) ([]byte, error) {
//	var block cipher.Block
//	var err error
//	blockSize := 8
//
//	if len(key)%blockSize != 0 {
//		key = ZeroPadding(key, blockSize)
//		if len(key) > 8 {
//			key = key[:8]
//		}
//	}
//
//	if isTripleDES {
//		block, err = des.NewTripleDESCipher(key)
//	} else {
//		block, err = des.NewCipher(key)
//	}
//	if err != nil {
//		return nil, fmt.Errorf("DES ECB Error: %s", err)
//	}
//	if len(data)%blockSize != 0 {
//		data = ZeroPadding(data, blockSize)
//	}
//
//	decrypted := make([]byte, len(data))
//	for bs, be := 0, blockSize; bs < len(data); bs, be = bs+blockSize, be+blockSize {
//		block.Decrypt(decrypted[bs:be], data[bs:be])
//	}
//	return decrypted, nil
//}
//
//func DESCBCEnc(key []byte, data []byte, iv []byte) ([]byte, error) {
//	return DESCBCEncEx(key, data, iv, false)
//}
//
//func TripleDES_CBCEnc(key []byte, data []byte, iv []byte) ([]byte, error) {
//	return DESCBCEncEx(key, data, iv, true)
//}
//
//func DESCBCDec(key []byte, data []byte, iv []byte) ([]byte, error) {
//	return DESCBCDecEx(key, data, iv, false)
//
//}
//func TripleDES_CBCDec(key []byte, data []byte, iv []byte) ([]byte, error) {
//	return DESCBCDecEx(key, data, iv, true)
//}

func DESECBEnc(key []byte, data []byte) ([]byte, error) {
	return DESEncryptECBWithZeroPadding(key, data, nil)
}

func TripleDES_ECBEnc(key []byte, data []byte) ([]byte, error) {
	return TripleDESEncryptECBWithZeroPadding(key, data, nil)
}

func DESECBDec(key []byte, data []byte) ([]byte, error) {
	return DESDecryptECBWithZeroPadding(key, data, nil)
}

func TripleDES_ECBDec(key []byte, data []byte) ([]byte, error) {
	return TripleDESDecryptECBWithZeroPadding(key, data, nil)
}

// Des
var DESEncryptCBCWithPKCSPadding = DESEncFactory(PKCS5Padding, CBC)
var DESEncryptCBCWithZeroPadding = DESEncFactory(ZeroPadding, CBC)
var DESDecryptCBCWithPKCSPadding = DESDecFactory(PKCS5UnPadding, CBC)
var DESDecryptCBCWithZeroPadding = DESDecFactory(ZeroUnPadding, CBC)

var DESEncryptECBWithPKCSPadding = DESEncFactory(PKCS5Padding, ECB)
var DESEncryptECBWithZeroPadding = DESEncFactory(ZeroPadding, ECB)
var DESDecryptECBWithPKCSPadding = DESDecFactory(PKCS5UnPadding, ECB)
var DESDecryptECBWithZeroPadding = DESDecFactory(ZeroUnPadding, ECB)

// TripleDes
var TripleDESEncryptCBCWithPKCSPadding = TripleDESEncFactory(PKCS5Padding, CBC)
var TripleDESEncryptCBCWithZeroPadding = TripleDESEncFactory(ZeroPadding, CBC)
var TripleDESDecryptCBCWithPKCSPadding = TripleDESDecFactory(PKCS5UnPadding, CBC)
var TripleDESDecryptCBCWithZeroPadding = TripleDESDecFactory(ZeroUnPadding, CBC)

var TripleDESEncryptECBWithPKCSPadding = TripleDESEncFactory(PKCS5Padding, ECB)
var TripleDESEncryptECBWithZeroPadding = TripleDESEncFactory(ZeroPadding, ECB)
var TripleDESDecryptECBWithPKCSPadding = TripleDESDecFactory(PKCS5UnPadding, ECB)
var TripleDESDecryptECBWithZeroPadding = TripleDESDecFactory(ZeroUnPadding, ECB)

func DESEncFactory(paddingFunc func([]byte, int) []byte, mode string) SymmetricCryptFunc {
	return func(key []byte, i interface{}, iv []byte) ([]byte, error) {
		data := paddingFunc(interfaceToBytes(i), 8)
		iv = FixIV(iv, key, 8)
		return DESEnc(key, data, iv, mode)
	}
}

func DESDecFactory(unpaddingFunc func([]byte) []byte, mode string) SymmetricCryptFunc {
	return func(key []byte, i interface{}, iv []byte) ([]byte, error) {
		iv = FixIV(iv, key, 8)
		res, err := DESDec(key, interfaceToBytes(i), iv, mode)
		if err != nil {
			return nil, err
		}
		return unpaddingFunc(res), nil
	}
}

func TripleDESEncFactory(paddingFunc func([]byte, int) []byte, mode string) SymmetricCryptFunc {
	return func(key []byte, i interface{}, iv []byte) ([]byte, error) {
		data := paddingFunc(interfaceToBytes(i), 8)
		iv = FixIV(iv, key, 8)
		return TripleDesEnc(key, data, iv, mode)
	}
}

func TripleDESDecFactory(unpaddingFunc func([]byte) []byte, mode string) SymmetricCryptFunc {
	return func(key []byte, i interface{}, iv []byte) ([]byte, error) {
		iv = FixIV(iv, key, 8)
		res, err := TripleDesDec(key, interfaceToBytes(i), iv, mode)
		if err != nil {
			return nil, err
		}
		return unpaddingFunc(res), nil
	}
}

func DESEnc(key []byte, data []byte, iv []byte, mode string) ([]byte, error) {
	data = ZeroPadding(data, 8)
	c, err := des.NewCipher(key)
	if err != nil {
		return nil, err
	}
	switch mode {
	case CBC:
		return CBCEncode(c, iv, data)
	case ECB:
		return ECBEncode(c, data)
	case CFB:
		return CFBEncode(c, iv, data)
	case OFB:
		return OFBEncode(c, iv, data)
	case CTR:
		return CTREncode(c, iv, data)
	default:
		return nil, fmt.Errorf("DES: invalid mode %s", mode)
	}
}

func DESDec(key []byte, data []byte, iv []byte, mode string) ([]byte, error) {
	c, err := des.NewCipher(key)
	if err != nil {
		return nil, err
	}
	switch mode {
	case CBC:
		return CBCDecode(c, iv, data)
	case ECB:
		return ECBDecode(c, data)
	case CFB:
		return CFBDecode(c, iv, data)
	case OFB:
		return OFBDecode(c, iv, data)
	case CTR:
		return CTRDecode(c, iv, data)
	default:
		return nil, fmt.Errorf("DES: invalid mode %s", mode)
	}
}

func TripleDesEnc(key []byte, data []byte, iv []byte, mode string) ([]byte, error) {
	data = ZeroPadding(data, 8)
	c, err := des.NewTripleDESCipher(key)
	if err != nil {
		return nil, err
	}
	switch mode {
	case CBC:
		return CBCEncode(c, iv, data)
	case ECB:
		return ECBEncode(c, data)
	case CFB:
		return CFBEncode(c, iv, data)
	case OFB:
		return OFBEncode(c, iv, data)
	case CTR:
		return CTREncode(c, iv, data)
	default:
		return nil, fmt.Errorf("TripleDES: invalid mode %s", mode)
	}
}

func TripleDesDec(key []byte, data []byte, iv []byte, mode string) ([]byte, error) {
	c, err := des.NewTripleDESCipher(key)
	if err != nil {
		return nil, err
	}
	switch mode {
	case CBC:
		return CBCDecode(c, iv, data)
	case ECB:
		return ECBDecode(c, data)
	case CFB:
		return CFBDecode(c, iv, data)
	case OFB:
		return OFBDecode(c, iv, data)
	case CTR:
		return CTRDecode(c, iv, data)
	default:
		return nil, fmt.Errorf("TripleDES: invalid mode %s", mode)
	}
}
