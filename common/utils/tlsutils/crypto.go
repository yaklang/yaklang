package tlsutils

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"reflect"
	"strings"
)

func Encrypt(raw []byte, pemBytes []byte) (string, error) {
	b, _ := pem.Decode(pemBytes)
	pub, err := x509.ParsePKIXPublicKey(b.Bytes)
	if err != nil {
		return "", utils.Errorf("parse public key failed: %s", err)
	}

	pubKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return "", utils.Errorf("parse pubkey[%s] need *rsa.PublicKey", reflect.TypeOf(pubKey))
	}

	var enc []string
	subs, err := SplitBlock(raw, (pubKey.Size()-11)/2)
	if err != nil {
		return "", utils.Errorf("split block failed: %s", err)
	}
	for _, sub := range subs {
		rs, err := rsa.EncryptPKCS1v15(cryptorand.Reader, pubKey, []byte(sub))
		if err != nil {
			return "", utils.Errorf("enc sub[%s] failed: %s", sub, err)
		}
		enc = append(enc, codec.EncodeToHex(rs))
	}
	return strings.Join(enc, "\n"), nil
}

func Decrypt(r string, priPem []byte) ([]byte, error) {
	b, _ := pem.Decode(priPem)
	pri, err := x509.ParsePKCS1PrivateKey(b.Bytes)
	if err != nil {
		return nil, utils.Errorf("parse public key failed: %s", err)
	}

	var groups []string
	for line := range utils.ParseLines(r) {
		lRaw, err := codec.DecodeHex(line)
		if err != nil {
			return nil, utils.Errorf("parse hex failed: %s", err)
		}

		res, err := rsa.DecryptPKCS1v15(cryptorand.Reader, pri, lRaw)
		if err != nil {
			return nil, utils.Errorf("dec block[%s] failed: %s", codec.StrConvQuote(string(lRaw)), err)
		}

		groups = append(groups, string(res))
	}

	return MergeBlock(groups)
}

func GeneratePrivateAndPublicKeyPEM() (pri []byte, pub []byte, _ error) {
	return GeneratePrivateAndPublicKeyPEMWithPrivateFormatter("pkcs#1")
}

func GeneratePrivateAndPublicKeyPEMWithPrivateFormatter(t string) (pri []byte, pub []byte, _ error) {
	pk, err := rsa.GenerateKey(cryptorand.Reader, 4096)
	if err != nil {
		return
	}

	var priBuffer bytes.Buffer
	var priDer []byte
	switch strings.ToLower(t) {
	case "pkcs1", "pkcs#1":
		priDer = x509.MarshalPKCS1PrivateKey(pk)
	case "ec", "ecdsa":
		pk, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
		if err != nil {
			return nil, nil, err
		}
		priDer, err = x509.MarshalECPrivateKey(pk)
		if err != nil {
			return nil, nil, utils.Errorf("marshal ecdsa prikey failed: %s", err)
		}
	case "pkcs8", "pkcs#8":
		fallthrough
	default:
		priDer, err = x509.MarshalPKCS8PrivateKey(pk)
		if err != nil {
			return nil, nil, utils.Errorf("marshal pkcs8 prikey failed: %s", err)
		}
	}
	err = pem.Encode(&priBuffer, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: priDer,
	})
	if err != nil {
		return nil, nil, utils.Errorf("marshal prikey failed: %s", err)
	}

	var pubBuffer bytes.Buffer
	pubDir, err := x509.MarshalPKIXPublicKey(pk.Public())
	if err != nil {
		return nil, nil, utils.Errorf("marshal pubkey failed: %s", err)
	}
	err = pem.Encode(&pubBuffer, &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: pubDir,
	})
	if err != nil {
		return nil, nil, utils.Errorf("marshal pem pubkey failed: %s", err)
	}

	return priBuffer.Bytes(), pubBuffer.Bytes(), nil
}

func PemPkcs1v15Encrypt(pemBytes []byte, data interface{}) ([]byte, error) {
	dataBytes := utils.InterfaceToBytes(data)
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.Wrap(errors.New("empty pem block"), "pem decode public key failed")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, `x509.ParsePKIXPublicKey(block.Bytes) failed`)
	}

	pubKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.Wrap(err, "need *rsp.PublicKey, cannot found! ")
	}
	_, _ = dataBytes, pub

	results, err := rsa.EncryptPKCS1v15(cryptorand.Reader, pubKey, dataBytes)
	if err != nil {
		return nil, errors.Wrap(err, `rsa.EncryptPKCS1v15(cryptorand.Reader, pubKey, dataBytes) error`)
	}
	return results, err
}

func PemPkcsOAEPEncrypt(pemBytes []byte, data interface{}) ([]byte, error) {
	dataBytes := utils.InterfaceToBytes(data)
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.Wrap(errors.New("empty pem block"), "pem decode public key failed")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, `x509.ParsePKIXPublicKey(block.Bytes) failed`)
	}

	pubKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.Wrap(err, "need *rsp.PublicKey, cannot found! ")
	}
	_, _ = dataBytes, pub

	results, err := rsa.EncryptOAEP(sha256.New(), cryptorand.Reader, pubKey, dataBytes, nil)
	if err != nil {
		return nil, errors.Wrap(err, `rsa.EncryptOAEP(cryptorand.Reader, pubKey, dataBytes) error`)
	}
	return results, err
}

func PemPkcsOAEPDecrypt(pemPriBytes []byte, data interface{}) ([]byte, error) {
	dataBytes := utils.InterfaceToBytes(data)
	b, _ := pem.Decode(pemPriBytes)
	pri, err := x509.ParsePKCS1PrivateKey(b.Bytes)
	if err != nil {
		parsedPri, err := x509.ParsePKCS8PrivateKey(b.Bytes)
		if err != nil {
			return nil, utils.Errorf("parse private key failed: %s", err)
		}

		var ok bool
		pri, ok = parsedPri.(*rsa.PrivateKey)
		if !ok {
			return nil, utils.Errorf("need *rsa.PrivateKey, cannot found! ")
		}

		if pri == nil {
			return nil, utils.Errorf("need *rsa.PrivateKey, cannot found! ")
		}
	}

	results, err := rsa.DecryptOAEP(sha256.New(), cryptorand.Reader, pri, dataBytes, nil)
	if err != nil {
		return nil, errors.Wrap(err, `rsa.PemPkcsOAEPDecrypt(cryptorand.Reader, pri, dataBytes) error`)
	}
	return results, err
}

func PemPkcs1v15Decrypt(pemPriBytes []byte, data interface{}) ([]byte, error) {
	dataBytes := utils.InterfaceToBytes(data)
	b, _ := pem.Decode(pemPriBytes)
	pri, err := x509.ParsePKCS1PrivateKey(b.Bytes)
	if err != nil {
		parsedPri, err := x509.ParsePKCS8PrivateKey(b.Bytes)
		if err != nil {
			return nil, utils.Errorf("parse private key failed: %s", err)
		}

		var ok bool
		pri, ok = parsedPri.(*rsa.PrivateKey)
		if !ok {
			return nil, utils.Errorf("need *rsa.PrivateKey, cannot found! ")
		}
	}

	results, err := rsa.DecryptPKCS1v15(cryptorand.Reader, pri, dataBytes)
	if err != nil {
		return nil, errors.Wrap(err, `rsa.DecryptPKCS1v15(cryptorand.Reader, pri, dataBytes) error`)
	}
	return results, err
}
