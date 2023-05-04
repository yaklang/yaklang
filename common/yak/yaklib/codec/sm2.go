package codec

import (
	"bytes"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/gmsm/sm2"
	"github.com/yaklang/yaklang/common/gmsm/x509"
)

import (
	cryptoRand "crypto/rand"
)

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
	pkey, err := sm2.GenerateKey(cryptoRand.Reader)
	if err != nil {
		return nil, nil, errors.Wrap(err, "sm2.GenerateKey(cryptoRand.Reader)")
	}
	pKeyBytes := []byte(x509.WritePrivateKeyToHex(pkey))
	pubKeyBytes := []byte(x509.WritePublicKeyToHex(pkey.Public().(*sm2.PublicKey)))
	return pKeyBytes, pubKeyBytes, nil
}

func SM2EncryptC1C2C3(pubKey []byte, data []byte) ([]byte, error) {
	//x509.ReadPublicKeyFromHex()
	var pub *sm2.PublicKey
	var err error
	if bytes.HasPrefix(pubKey, []byte("---")) {
		pub, err = x509.ReadPublicKeyFromPem(pubKey)
	} else {
		pub, err = x509.ReadPublicKeyFromHex(string(pubKey))
	}
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
	var pri *sm2.PrivateKey
	var err error
	if bytes.HasPrefix(priKey, []byte("---")) {
		pri, err = x509.ReadPrivateKeyFromPem(priKey, password)
	} else {
		pri, err = x509.ReadPrivateKeyFromHex(string(priKey))
	}
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.privateKey")
	}

	results, err := sm2.Decrypt(pri, data, sm2.C1C2C3)
	if err != nil {
		return nil, errors.Wrap(err, "sm2.Decrypt[C1C2C3] with prikey")
	}
	return results, nil
}

func SM2EncryptC1C3C2(pubKey []byte, data []byte) ([]byte, error) {
	//x509.ReadPublicKeyFromHex()
	var pub *sm2.PublicKey
	var err error
	if bytes.HasPrefix(pubKey, []byte("---")) {
		pub, err = x509.ReadPublicKeyFromPem(pubKey)
	} else {
		pub, err = x509.ReadPublicKeyFromHex(string(pubKey))
	}
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
	var pri *sm2.PrivateKey
	var err error
	if bytes.HasPrefix(priKey, []byte("---")) {
		pri, err = x509.ReadPrivateKeyFromPem(priKey, password)
	} else {
		pri, err = x509.ReadPrivateKeyFromHex(string(priKey))
	}
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.privateKey")
	}

	results, err := sm2.Decrypt(pri, data, sm2.C1C3C2)
	if err != nil {
		return nil, errors.Wrap(err, "sm2.Decrypt[C1C3C2] with pubkey")
	}
	return results, nil
}

func SM2EncryptASN1(pubKey []byte, data []byte) ([]byte, error) {
	var pub *sm2.PublicKey
	var err error
	if bytes.HasPrefix(pubKey, []byte("---")) {
		pub, err = x509.ReadPublicKeyFromPem(pubKey)
	} else {
		pub, err = x509.ReadPublicKeyFromHex(string(pubKey))
	}
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.publicKey")
	}

	return sm2.EncryptAsn1(pub, data, cryptoRand.Reader)
}

func SM2DecryptASN1(priKey []byte, data []byte) ([]byte, error) {
	return SM2DecryptASN1WithPassword(priKey, data, nil)
}

func SM2DecryptASN1WithPassword(priKey []byte, data []byte, password []byte) ([]byte, error) {
	var pri *sm2.PrivateKey
	var err error
	if bytes.HasPrefix(priKey, []byte("---")) {
		pri, err = x509.ReadPrivateKeyFromPem(priKey, password)
	} else {
		pri, err = x509.ReadPrivateKeyFromHex(string(priKey))
	}
	if err != nil {
		return nil, errors.Wrap(err, "read sm2.privateKey")
	}

	return sm2.DecryptAsn1(pri, data)
}
