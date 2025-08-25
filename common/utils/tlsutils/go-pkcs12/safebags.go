// Copyright 2015, 2018, 2019 Opsmate, Inc. All rights reserved.
// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkcs12

import (
	"crypto/dsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"io"
	"math/big"

	x509gm "github.com/yaklang/yaklang/common/gmsm/x509"
)

var (
	// see https://tools.ietf.org/html/rfc7292#appendix-D
	oidCertTypeX509Certificate = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 9, 22, 1})
	oidKeyBag                  = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 12, 10, 1, 1})
	oidPKCS8ShroundedKeyBag    = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 12, 10, 1, 2})
	oidCertBag                 = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 12, 10, 1, 3})
	oidDSA                     = asn1.ObjectIdentifier([]int{1, 2, 840, 10040, 4, 1}) // DSA OID
)

// DSA私钥PKCS8结构
type pkcs8DSAPrivateKey struct {
	Version    int
	Algorithm  pkix.AlgorithmIdentifier
	PrivateKey []byte
}

// DSA参数结构
type dsaAlgorithmParameters struct {
	P, Q, G *big.Int
}

// DSA私钥结构
type dsaPrivateKey struct {
	X *big.Int
}

type certBag struct {
	Id   asn1.ObjectIdentifier
	Data []byte `asn1:"tag:0,explicit"`
}

func decodePkcs8ShroudedKeyBag(asn1Data, password []byte) (privateKey interface{}, err error) {
	pkinfo := new(encryptedPrivateKeyInfo)
	if err = unmarshal(asn1Data, pkinfo); err != nil {
		return nil, errors.New("pkcs12: error decoding PKCS#8 shrouded key bag: " + err.Error())
	}

	pkData, err := pbDecrypt(pkinfo, password)
	if err != nil {
		return nil, errors.New("pkcs12: error decrypting PKCS#8 shrouded key bag: " + err.Error())
	}

	// 先尝试标准解析
	privateKey, err = x509.ParsePKCS8PrivateKey(pkData)
	if err != nil {
		// 检查是否是DSA错误
		if err.Error() == "x509: PKCS#8 wrapping contained private key with unknown algorithm: 1.2.840.10040.4.1" {
			return parseDSAPrivateKey(pkData)
		}
		if err.Error() == "x509: failed to parse EC private key embedded in PKCS#8: x509: unknown elliptic curve" {
			return x509gm.ParsePKCS8PrivateKey(pkData, nil)
		}
		return nil, errors.New("pkcs12: error parsing PKCS#8 private key: " + err.Error())
	}

	return privateKey, nil
}

// 解析DSA私钥
func parseDSAPrivateKey(data []byte) (*dsa.PrivateKey, error) {
	var privKey pkcs8DSAPrivateKey
	if _, err := asn1.Unmarshal(data, &privKey); err != nil {
		return nil, errors.New("pkcs12: error unmarshaling DSA private key: " + err.Error())
	}

	if !privKey.Algorithm.Algorithm.Equal(oidDSA) {
		return nil, errors.New("pkcs12: not a DSA private key")
	}

	// 解析DSA参数
	var params dsaAlgorithmParameters
	if _, err := asn1.Unmarshal(privKey.Algorithm.Parameters.FullBytes, &params); err != nil {
		return nil, errors.New("pkcs12: error unmarshaling DSA parameters: " + err.Error())
	}

	// 从私钥字节中提取X值（简单处理，不需要复杂的ASN.1解析）
	x := new(big.Int).SetBytes(privKey.PrivateKey)

	// 构造DSA私钥
	key := &dsa.PrivateKey{
		PublicKey: dsa.PublicKey{
			Parameters: dsa.Parameters{
				P: params.P,
				Q: params.Q,
				G: params.G,
			},
		},
		X: x,
	}

	// 计算Y值
	key.Y = new(big.Int).Exp(key.Parameters.G, key.X, key.Parameters.P)

	return key, nil
}

func encodePkcs8ShroudedKeyBag(rand io.Reader, privateKey interface{}, algoID asn1.ObjectIdentifier, password []byte, iterations int, saltLen int) (asn1Data []byte, err error) {
	var pkData []byte
	if pkData, err = x509gm.MarshalPKCS8PrivateKey(privateKey); err != nil {
		return nil, errors.New("pkcs12: error encoding PKCS#8 private key: " + err.Error())
	}

	randomSalt := make([]byte, saltLen)
	if _, err = rand.Read(randomSalt); err != nil {
		return nil, errors.New("pkcs12: error reading random salt: " + err.Error())
	}

	var paramBytes []byte
	if algoID.Equal(oidPBES2) {
		if paramBytes, err = makePBES2Parameters(rand, randomSalt, iterations); err != nil {
			return nil, errors.New("pkcs12: error encoding params: " + err.Error())
		}
	} else {
		if paramBytes, err = asn1.Marshal(pbeParams{Salt: randomSalt, Iterations: iterations}); err != nil {
			return nil, errors.New("pkcs12: error encoding params: " + err.Error())
		}
	}

	var pkinfo encryptedPrivateKeyInfo
	pkinfo.AlgorithmIdentifier.Algorithm = algoID
	pkinfo.AlgorithmIdentifier.Parameters.FullBytes = paramBytes

	if err = pbEncrypt(&pkinfo, pkData, password); err != nil {
		return nil, errors.New("pkcs12: error encrypting PKCS#8 shrouded key bag: " + err.Error())
	}

	if asn1Data, err = asn1.Marshal(pkinfo); err != nil {
		return nil, errors.New("pkcs12: error encoding PKCS#8 shrouded key bag: " + err.Error())
	}

	return asn1Data, nil
}

func decodeCertBag(asn1Data []byte) (x509Certificates []byte, err error) {
	bag := new(certBag)
	if err := unmarshal(asn1Data, bag); err != nil {
		return nil, errors.New("pkcs12: error decoding cert bag: " + err.Error())
	}
	if !bag.Id.Equal(oidCertTypeX509Certificate) {
		return nil, NotImplementedError("only X509 certificates are supported in cert bags")
	}
	return bag.Data, nil
}

func encodeCertBag(x509Certificates []byte) (asn1Data []byte, err error) {
	var bag certBag
	bag.Id = oidCertTypeX509Certificate
	bag.Data = x509Certificates
	if asn1Data, err = asn1.Marshal(bag); err != nil {
		return nil, errors.New("pkcs12: error encoding cert bag: " + err.Error())
	}
	return asn1Data, nil
}
