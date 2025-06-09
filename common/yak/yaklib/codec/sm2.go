package codec

import (
	"bytes"
	"math/big"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/gmsm/sm2"
	"github.com/yaklang/yaklang/common/gmsm/x509"

	cryptoRand "crypto/rand"
)

// isHexString 检查字节数组是否为hex字符串格式
func isHexString(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	// 检查是否全部为hex字符
	for _, b := range data {
		if !((b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')) {
			return false
		}
	}
	return true
}

// parsePrivateKey 解析私钥，支持PEM格式、hex字符串和已解码的字节数组
func parsePrivateKey(priKey []byte, password []byte) (*sm2.PrivateKey, error) {
	if bytes.HasPrefix(priKey, []byte("---")) {
		// PEM格式
		return x509.ReadPrivateKeyFromPem(priKey, password)
	} else if isHexString(priKey) {
		// hex字符串格式
		return x509.ReadPrivateKeyFromHex(string(priKey))
	} else {
		// 已解码的字节数组，直接构造私钥
		// 对于SM2私钥，如果是32字节的原始字节数组，直接使用
		if len(priKey) == 32 {
			return readPrivateKeyFromBytes(priKey)
		}
		// 如果不是32字节，可能是其他格式，尝试hex解析
		return x509.ReadPrivateKeyFromHex(string(priKey))
	}
}

// parsePublicKey 解析公钥，支持PEM格式、hex字符串和已解码的字节数组
func parsePublicKey(pubKey []byte) (*sm2.PublicKey, error) {
	pubKey = bytes.TrimSpace(pubKey)
	if bytes.HasPrefix(pubKey, []byte("---")) {
		// PEM格式
		return x509.ReadPublicKeyFromPem(pubKey)
	} else if isHexString(pubKey) {
		// hex字符串格式
		return x509.ReadPublicKeyFromHex(string(pubKey))
	} else {
		// 已解码的字节数组，直接构造公钥
		// 对于SM2公钥，如果是64或65字节的原始字节数组，直接使用
		if len(pubKey) == 64 || len(pubKey) == 65 {
			return readPublicKeyFromBytes(pubKey)
		}
		// 如果不是预期长度，可能是其他格式，尝试hex解析
		return x509.ReadPublicKeyFromHex(string(pubKey))
	}
}

// readPrivateKeyFromBytes 从字节数组构造SM2私钥
func readPrivateKeyFromBytes(d []byte) (*sm2.PrivateKey, error) {
	c := sm2.P256Sm2()
	k := new(big.Int).SetBytes(d)
	params := c.Params()
	one := new(big.Int).SetInt64(1)
	n := new(big.Int).Sub(params.N, one)
	if k.Cmp(n) >= 0 {
		return nil, errors.New("privateKey's D is overflow.")
	}
	priv := new(sm2.PrivateKey)
	priv.PublicKey.Curve = c
	priv.D = k
	priv.PublicKey.X, priv.PublicKey.Y = c.ScalarBaseMult(k.Bytes())
	return priv, nil
}

// readPublicKeyFromBytes 从字节数组构造SM2公钥
func readPublicKeyFromBytes(q []byte) (*sm2.PublicKey, error) {
	if len(q) == 65 && q[0] == byte(0x04) {
		q = q[1:]
	}
	if len(q) != 64 {
		return nil, errors.New("publicKey is not uncompressed.")
	}
	pub := new(sm2.PublicKey)
	pub.Curve = sm2.P256Sm2()
	pub.X = new(big.Int).SetBytes(q[:32])
	pub.Y = new(big.Int).SetBytes(q[32:])
	return pub, nil
}

func GenerateSM2PrivateKeyPEM() ([]byte, []byte, error) {
	pkey, err := sm2.GenerateKey(cryptoRand.Reader)
	if err != nil {
		return nil, nil, errors.Wrap(err, "sm2.GenerateKey(cryptoRand.Reader)")
	}
	pKeyBytes, err := x509.WritePrivateKeyToPem(pkey, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "write sm2.privateKey to pem")
	}

	pubKeyBytes, err := x509.WritePublicKeyToPem(pkey.Public().(*sm2.PublicKey))
	if err != nil {
		return nil, nil, errors.Wrap(err, "write sm2.publicKey to pem")
	}
	return pKeyBytes, pubKeyBytes, nil
}

func GenerateSM2PrivateKeyHEX() ([]byte, []byte, error) {
	for i := 0; i < 10; i++ {
		pkey, err := sm2.GenerateKey(cryptoRand.Reader)
		if err != nil {
			return nil, nil, errors.Wrap(err, "sm2.GenerateKey(cryptoRand.Reader)")
		}
		pKeyBytes := []byte(x509.WritePrivateKeyToHex(pkey))
		pubKeyBytes := []byte(x509.WritePublicKeyToHex(pkey.Public().(*sm2.PublicKey)))

		data, err := SM2EncryptC1C2C3(pubKeyBytes, []byte("abc"))
		if err != nil {
			continue
		}
		decData, err := SM2DecryptC1C2C3(pKeyBytes, data)
		if err != nil {
			continue
		}
		if string(decData) != "abc" {
			continue
		}
		return pKeyBytes, pubKeyBytes, nil
	}

	return nil, nil, errors.New("generate sm2 private key failed")
}

// preprocessCiphertext 预处理密文，自动添加缺失的0x04前缀
func preprocessCiphertext(data []byte) []byte {
	// 如果密文长度符合无前缀格式（96 + 密文数据长度），且第一个字节不是0x04
	if len(data) >= 96 && data[0] != 0x04 {
		// 自动添加0x04前缀
		result := make([]byte, len(data)+1)
		result[0] = 0x04
		copy(result[1:], data)
		return result
	}
	return data
}

func SM2EncryptC1C2C3(pubKey []byte, data []byte) ([]byte, error) {
	pub, err := parsePublicKey(pubKey)
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.publicKey")
	}

	results, err := sm2.Encrypt(pub, data, cryptoRand.Reader, sm2.C1C2C3)
	if err != nil {
		return nil, errors.Wrap(err, "sm2.Encrypt[C1C2C3] with pubkey")
	}
	return results, nil
}

func SM2DecryptC1C2C3(priKey []byte, data []byte) ([]byte, error) {
	return SM2DecryptC1C2C3WithPassword(priKey, data, nil)
}

func SM2DecryptC1C2C3WithPassword(priKey []byte, data []byte, password []byte) ([]byte, error) {
	pri, err := parsePrivateKey(priKey, password)
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.privateKey")
	}

	// 预处理密文，自动添加缺失的0x04前缀
	processedData := preprocessCiphertext(data)

	results, err := sm2.Decrypt(pri, processedData, sm2.C1C2C3)
	if err != nil {
		return nil, errors.Wrap(err, "sm2.Decrypt[C1C2C3] with prikey")
	}
	return results, nil
}

func SM2EncryptC1C3C2(pubKey []byte, data []byte) ([]byte, error) {
	pub, err := parsePublicKey(pubKey)
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.publicKey")
	}

	results, err := sm2.Encrypt(pub, data, cryptoRand.Reader, sm2.C1C3C2)
	if err != nil {
		return nil, errors.Wrap(err, "sm2.Encrypt[C1C3C2] with pubkey")
	}
	return results, nil
}

func SM2DecryptC1C3C2(priKey []byte, data []byte) ([]byte, error) {
	return SM2DecryptC1C3C2WithPassword(priKey, data, nil)
}

func SM2DecryptC1C3C2WithPassword(priKey []byte, data []byte, password []byte) ([]byte, error) {
	pri, err := parsePrivateKey(priKey, password)
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.privateKey")
	}

	// 预处理密文，自动添加缺失的0x04前缀
	processedData := preprocessCiphertext(data)

	results, err := sm2.Decrypt(pri, processedData, sm2.C1C3C2)
	if err != nil {
		return nil, errors.Wrap(err, "sm2.Decrypt[C1C3C2] with pubkey")
	}
	return results, nil
}

func SM2EncryptASN1(pubKey []byte, data []byte) ([]byte, error) {
	pub, err := parsePublicKey(pubKey)
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.publicKey")
	}

	return sm2.EncryptAsn1(pub, data, cryptoRand.Reader)
}

func SM2DecryptASN1(priKey []byte, data []byte) ([]byte, error) {
	return SM2DecryptASN1WithPassword(priKey, data, nil)
}

func SM2DecryptASN1WithPassword(priKey []byte, data []byte, password []byte) ([]byte, error) {
	pri, err := parsePrivateKey(priKey, password)
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.privateKey")
	}

	return sm2.DecryptAsn1(pri, data)
}
