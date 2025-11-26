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

func PKCS7PaddingFor8ByteBlock(src []byte) []byte {
	return PKCS5Padding(src, 8)
}

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
	if err := BlockCheck(iv, data, c.BlockSize()); err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	encrypter := cipher.NewCFBEncrypter(c, iv)
	encrypter.XORKeyStream(out, data)
	return out, nil
}

func CFBDecode(c cipher.Block, iv, data []byte) ([]byte, error) {
	if err := BlockCheck(iv, data, c.BlockSize()); err != nil {
		return nil, err
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
	if err := BlockCheck(iv, data, c.BlockSize()); err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	encrypter := cipher.NewOFB(c, iv)
	encrypter.XORKeyStream(out, data)
	return out, nil
}

func OFBDecode(c cipher.Block, iv, data []byte) ([]byte, error) {
	if err := BlockCheck(iv, data, c.BlockSize()); err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	decrypter := cipher.NewOFB(c, iv)
	decrypter.XORKeyStream(out, data)
	return out, nil
}

func CTREncode(c cipher.Block, iv, data []byte) ([]byte, error) {
	if err := BlockCheck(iv, data, c.BlockSize()); err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	encrypter := cipher.NewCTR(c, iv)
	encrypter.XORKeyStream(out, data)
	return out, nil
}

func CTRDecode(c cipher.Block, iv, data []byte) ([]byte, error) {
	if err := BlockCheck(iv, data, c.BlockSize()); err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	decrypter := cipher.NewCTR(c, iv)
	decrypter.XORKeyStream(out, data)
	return out, nil
}

type EncodedFunc func(any) string

func RandBytes(n int) []byte {
	random := make([]byte, n)
	_, err := io.ReadFull(cryptoRand.Reader, random)
	if err != nil {
		log.Errorf("failed to read random bytes: %v", err)
		return nil
	}
	return random
}
