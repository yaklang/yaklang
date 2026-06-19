package codec

import (
	"crypto/rc4"
)

// RC4Decrypt 使用 RC4 流密码解密数据(RC4 加解密为同一运算)
// 参数:
//   - cipherKey: RC4 密钥(长度可变)
//   - cipherText: 待解密的密文字节
//
// 返回值:
//   - []byte: 解密还原后的明文字节
//   - error: 密钥非法等错误
//
// Example:
// ```
// // VARS: 先加密再解密(RC4)
// key = "secretkey"
// ct = codec.RC4Encrypt(key, "Secret Message")~
// pt = codec.RC4Decrypt(key, ct)~
// // STDOUT: 打印还原后的明文
// println(string(pt))   // OUT: Secret Message
// // assert: 锁定结论(RC4 解密还原一致)
// assert string(pt) == "Secret Message", "RC4 decrypt should recover plaintext"
// ```
func RC4Decrypt(cipherKey []byte, cipherText []byte) ([]byte, error) {
	cipher, err := rc4.NewCipher(cipherKey)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(cipherText))
	cipher.XORKeyStream(plaintext, cipherText)
	return plaintext, nil
}

// RC4Encrypt 使用 RC4 流密码加密数据(RC4 加解密为同一运算)
// 参数:
//   - cipherKey: RC4 密钥(长度可变)
//   - plainText: 待加密的明文字节
//
// 返回值:
//   - []byte: 加密后的密文字节
//   - error: 密钥非法等错误
//
// Example:
// ```
// // VARS: RC4 加解密往返
// key = "secretkey"
// ct = codec.RC4Encrypt(key, "Secret Message")~
// // STDOUT: 解密还原后打印
// println(string(codec.RC4Decrypt(key, ct)~))   // OUT: Secret Message
// // assert: 锁定结论(RC4 加解密往返一致)
// assert string(codec.RC4Decrypt(key, ct)~) == "Secret Message", "RC4 encrypt/decrypt should round-trip"
// ```
func RC4Encrypt(cipherKey []byte, plainText []byte) ([]byte, error) {
	cipher, err := rc4.NewCipher(cipherKey)
	if err != nil {
		return nil, err
	}
	ciphertext := make([]byte, len(plainText))
	cipher.XORKeyStream(ciphertext, plainText)
	return ciphertext, nil
}
