package codec

import (
	"crypto/cipher"
	cryptoRand "crypto/rand"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/yaklang/yaklang/common/gmsm/sm4"
	"github.com/yaklang/yaklang/common/log"
)

var (
	CBC = "CBC"
	ECB = "ECB"
	CFB = "CFB"
	OFB = "OFB"
	CTR = "CTR"
)

var PKCS7Padding = sm4.PKCS7Padding
var PKCS7UnPadding = sm4.PKCS7UnPadding

// PKCS7PaddingFor8ByteBlock 按 8 字节块大小对数据做 PKCS7/PKCS5 填充(常用于 DES)
// 参数:
//   - src: 待填充的数据字节
//
// 返回值:
//   - 填充到 8 字节整数倍后的数据字节
//
// Example:
// ```
// // VARS: 对 3 字节数据按 8 字节块填充
// padded = codec.PKCS7PaddingForDES("abc")
// // STDOUT: 打印填充后的长度
// println(len(padded))   // OUT: 8
// // assert: 锁定结论(去填充可还原原始数据)
// assert string(codec.PKCS7UnPaddingForDES(padded)) == "abc", "PKCS7 for DES padding/unpadding should round-trip"
// ```
func PKCS7PaddingFor8ByteBlock(src []byte) []byte {
	return PKCS5Padding(src, 8)
}

// PKCS7UnPaddingFor8ByteBlock 去除 8 字节块大小的 PKCS7/PKCS5 填充(常用于 DES)
// 参数:
//   - src: 含 PKCS7 填充的数据字节
//
// 返回值:
//   - 去除填充后的原始数据字节
//
// Example:
// ```
// // VARS: 填充后再去填充往返
// padded = codec.PKCS7PaddingForDES("abc")
// unpadded = codec.PKCS7UnPaddingForDES(padded)
// // STDOUT: 打印去填充后的结果
// println(string(unpadded))   // OUT: abc
// // assert: 锁定结论(去填充还原原始数据)
// assert string(unpadded) == "abc", "PKCS7 for DES unpadding should recover original data"
// ```
func PKCS7UnPaddingFor8ByteBlock(src []byte) []byte {
	return PKCS5UnPadding(src)
}

func FixIV(iv, key []byte, blockSize int) []byte {
	if iv == nil && len(key) >= blockSize { // iv is nil, use key as iv
		iv = key[:blockSize]
	}
	if len(iv) > blockSize { // iv is too long, truncate it
		iv = iv[:blockSize]
	}
	return ZeroPadding(iv, blockSize)
}

func BlockCheck(iv, data []byte, blockSize int) error {
	if len(iv) != blockSize {
		return errors.New("check iv length: invalid iv size " + strconv.Itoa(len(iv)))
	}

	if len(data)%blockSize != 0 {
		return errors.New("check data length: invalid data size " + strconv.Itoa(len(data)))
	}

	return nil
}

func CBCEncode(c cipher.Block, iv, data []byte) ([]byte, error) {
	if err := BlockCheck(iv, data, c.BlockSize()); err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	encrypter := cipher.NewCBCEncrypter(c, iv)
	encrypter.CryptBlocks(out, data)
	return out, nil
}

func CBCDecode(c cipher.Block, iv, data []byte) ([]byte, error) {
	if err := BlockCheck(iv, data, c.BlockSize()); err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	decrypter := cipher.NewCBCDecrypter(c, iv)
	decrypter.CryptBlocks(out, data)
	return out, nil
}

func CFBEncode(c cipher.Block, iv, data []byte) ([]byte, error) {
	// CFB 是流模式，不需要数据对齐到块大小，只需要检查 IV 长度
	if len(iv) != c.BlockSize() {
		return nil, errors.New("check iv length: invalid iv size " + strconv.Itoa(len(iv)))
	}
	out := make([]byte, len(data))
	encrypter := cipher.NewCFBEncrypter(c, iv)
	encrypter.XORKeyStream(out, data)
	return out, nil
}

func CFBDecode(c cipher.Block, iv, data []byte) ([]byte, error) {
	// CFB 是流模式，不需要数据对齐到块大小，只需要检查 IV 长度
	if len(iv) != c.BlockSize() {
		return nil, errors.New("check iv length: invalid iv size " + strconv.Itoa(len(iv)))
	}
	out := make([]byte, len(data))
	decrypter := cipher.NewCFBDecrypter(c, iv)
	decrypter.XORKeyStream(out, data)
	return out, nil
}

func ECBEncode(c cipher.Block, data []byte) ([]byte, error) {
	blockSize := c.BlockSize()
	if len(data)%blockSize != 0 {
		return nil, fmt.Errorf("check data length: invalid data size %d, need %d", len(data), blockSize)
	}
	out := make([]byte, len(data))
	for i := 0; i < len(data)/blockSize; i++ {
		in_tmp := data[i*blockSize : i*blockSize+blockSize]
		out_tmp := make([]byte, blockSize)
		c.Encrypt(out_tmp, in_tmp)
		copy(out[i*blockSize:i*blockSize+blockSize], out_tmp)
	}
	return out, nil
}

func ECBDecode(c cipher.Block, data []byte) ([]byte, error) {
	blockSize := c.BlockSize()
	if len(data)%blockSize != 0 {
		return nil, fmt.Errorf("check data length: invalid data size %d, need %d", len(data), blockSize)
	}
	out := make([]byte, len(data))
	for i := 0; i < len(data)/blockSize; i++ {
		in_tmp := data[i*blockSize : i*blockSize+blockSize]
		out_tmp := make([]byte, blockSize)
		c.Decrypt(out_tmp, in_tmp)
		copy(out[i*blockSize:i*blockSize+blockSize], out_tmp)
	}
	return out, nil
}

func OFBEncode(c cipher.Block, iv, data []byte) ([]byte, error) {
	// OFB 是流模式，不需要数据对齐到块大小，只需要检查 IV 长度
	if len(iv) != c.BlockSize() {
		return nil, errors.New("check iv length: invalid iv size " + strconv.Itoa(len(iv)))
	}
	out := make([]byte, len(data))
	encrypter := cipher.NewOFB(c, iv)
	encrypter.XORKeyStream(out, data)
	return out, nil
}

func OFBDecode(c cipher.Block, iv, data []byte) ([]byte, error) {
	// OFB 是流模式，不需要数据对齐到块大小，只需要检查 IV 长度
	if len(iv) != c.BlockSize() {
		return nil, errors.New("check iv length: invalid iv size " + strconv.Itoa(len(iv)))
	}
	out := make([]byte, len(data))
	decrypter := cipher.NewOFB(c, iv)
	decrypter.XORKeyStream(out, data)
	return out, nil
}

func CTREncode(c cipher.Block, iv, data []byte) ([]byte, error) {
	// CTR 是流模式，不需要数据对齐到块大小，只需要检查 IV 长度
	if len(iv) != c.BlockSize() {
		return nil, errors.New("check iv length: invalid iv size " + strconv.Itoa(len(iv)))
	}
	out := make([]byte, len(data))
	encrypter := cipher.NewCTR(c, iv)
	encrypter.XORKeyStream(out, data)
	return out, nil
}

func CTRDecode(c cipher.Block, iv, data []byte) ([]byte, error) {
	// CTR 是流模式，不需要数据对齐到块大小，只需要检查 IV 长度
	if len(iv) != c.BlockSize() {
		return nil, errors.New("check iv length: invalid iv size " + strconv.Itoa(len(iv)))
	}
	out := make([]byte, len(data))
	decrypter := cipher.NewCTR(c, iv)
	decrypter.XORKeyStream(out, data)
	return out, nil
}

type EncodedFunc func(any) string

// RandBytes 生成 n 个密码学安全的随机字节
// 参数:
//   - n: 需要生成的随机字节数量
//
// 返回值:
//   - 长度为 n 的随机字节切片(读取失败时返回 nil)
//
// Example:
// ```
// // VARS: 生成 16 个随机字节(每次结果不同)
// result = codec.RandBytes(16)
// // STDOUT: 打印长度
// println(len(result))   // OUT: 16
// // assert: 锁定结论(长度固定为请求值)
// assert len(result) == 16, "RandBytes should produce requested length"
// ```
func RandBytes(n int) []byte {
	random := make([]byte, n)
	_, err := io.ReadFull(cryptoRand.Reader, random)
	if err != nil {
		log.Errorf("failed to read random bytes: %v", err)
		return nil
	}
	return random
}
