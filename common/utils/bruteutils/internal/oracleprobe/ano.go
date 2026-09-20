// Derived from go-ora v3.0.1 and master 360b4b7 advanced_nego: password auth, encryption and
// integrity services only. Kerberos/NTS are separate authentication methods.
package oracleprobe

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
)

type anoField struct {
	kind uint16
	data []byte
}

func anoNumber(kind uint16, n uint32, size int) anoField {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], n)
	return anoField{kind, bytes.Clone(b[4-size:])}
}
func anoService(id uint16, fields ...anoField) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint16(b, id)
	binary.BigEndian.PutUint16(b[2:], uint16(len(fields)))
	for _, f := range fields {
		b = binary.BigEndian.AppendUint16(b, uint16(len(f.data)))
		b = binary.BigEndian.AppendUint16(b, f.kind)
		b = append(b, f.data...)
	}
	return b
}
func (s *session) sendANO(services ...[]byte) error {
	b := []byte{0xde, 0xad, 0xbe, 0xef, 0, 0, 0x17, 0, 0, 0, 0, byte(len(services)), 0}
	for _, v := range services {
		b = append(b, v...)
	}
	binary.BigEndian.PutUint16(b[4:], uint16(len(b)))
	return s.writeData(b)
}
func (s *session) negotiateAdvanced() error {
	version := anoNumber(5, 0x17000000, 4)
	arr := []byte{0xde, 0xad, 0xbe, 0xef, 0, 3, 0, 0, 0, 4, 0, 4, 0, 1, 0, 2, 0, 3}
	e := s.sendANO(
		anoService(4, version, anoField{1, []byte{0, 0, 16, 28, 102, 236, 40, 234}}, anoField{1, arr}),
		anoService(1, version, anoNumber(3, 0xe0e1, 2), anoNumber(6, 0xfcff, 2)),
		anoService(2, version, anoField{1, []byte{0, 1, 8, 10, 6, 15, 16, 17}}, anoNumber(2, 1, 1)),
		anoService(3, version, anoField{1, []byte{0, 1, 3, 4, 5, 6}}),
	)
	if e != nil {
		return e
	}
	h, e := s.GetBytes(13)
	if e != nil {
		return e
	}
	if binary.BigEndian.Uint32(h) != 0xdeadbeef {
		return errors.New("oracle: invalid ANO header")
	}
	total := int(binary.BigEndian.Uint16(h[4:]))
	count := int(binary.BigEndian.Uint16(h[10:]))
	if total < 13 || total > maxField || count != 4 {
		return errors.New("oracle: invalid ANO response size")
	}
	consumed := 13
	seen := make(map[int]bool)
	encID, hashID := 0, 0
	encNew, hashNew := false, false
	var dh []anoField
	for i := 0; i < count; i++ {
		hdr, e := s.GetBytes(8)
		if e != nil {
			return e
		}
		consumed += 8
		id := int(binary.BigEndian.Uint16(hdr))
		n := int(binary.BigEndian.Uint16(hdr[2:]))
		code := int(binary.BigEndian.Uint32(hdr[4:]))
		if code != 0 {
			return &Error{Code: code, Message: "network negotiation failed"}
		}
		if id < 1 || id > 4 || seen[id] || n < 2 || n > 8 {
			return errors.New("oracle: invalid ANO service")
		}
		seen[id] = true
		f := make([]anoField, n)
		for j := range f {
			hdr, e := s.GetBytes(4)
			if e != nil {
				return e
			}
			size := int(binary.BigEndian.Uint16(hdr))
			consumed += 4 + size
			if consumed > total {
				return errors.New("oracle: ANO field exceeds response")
			}
			f[j].kind = binary.BigEndian.Uint16(hdr[2:])
			f[j].data, e = s.GetBytes(size)
			if e != nil {
				return e
			}
		}
		if f[0].kind != 5 || len(f[0].data) != 4 {
			return errors.New("oracle: invalid ANO version")
		}
		serviceVersion := binary.BigEndian.Uint32(f[0].data)
		newVersion := serviceVersion>>24 >= 23 || (serviceVersion>>12)&0xff == 1
		switch id {
		case 4:
			if n != 3 || f[1].kind != 6 || len(f[1].data) != 2 || binary.BigEndian.Uint16(f[1].data) != 31 {
				return errors.New("oracle: invalid ANO supervisor status")
			}
		case 1:
			if n != 2 || f[1].kind != 6 || len(f[1].data) != 2 || binary.BigEndian.Uint16(f[1].data) != 0xfbff {
				return errors.New("oracle: server requires non-password authentication")
			}
		case 2:
			if n != 2 || f[1].kind != 2 || len(f[1].data) != 1 {
				return errors.New("oracle: invalid encryption selection")
			}
			encID = int(f[1].data[0])
			encNew = newVersion && encID >= 15
		case 3:
			if (n != 2 && n != 8) || f[1].kind != 2 || len(f[1].data) != 1 {
				return errors.New("oracle: invalid integrity selection")
			}
			hashID = int(f[1].data[0])
			hashNew = newVersion && hashID != 1
			if n == 8 {
				dh = f[2:]
			}
		}
	}
	if consumed != total {
		return errors.New("oracle: ANO length mismatch")
	}
	if encID == 0 && hashID == 0 && dh == nil {
		return nil
	}
	if len(dh) != 6 || dh[0].kind != 3 || dh[1].kind != 3 || len(dh[0].data) != 2 || len(dh[1].data) != 2 {
		return errors.New("oracle: missing DH parameters")
	}
	bits := int(binary.BigEndian.Uint16(dh[0].data))
	primeBits := int(binary.BigEndian.Uint16(dh[1].data))
	n := (bits + 7) / 8
	if bits < 256 || bits > 4096 || primeBits < 1 || primeBits > 4096 || len(dh[3].data) != n || len(dh[4].data) != n || len(dh[2].data) > n || len(dh[5].data) > 256 {
		return errors.New("oracle: invalid DH lengths")
	}
	for _, f := range dh[2:] {
		if f.kind != 1 {
			return errors.New("oracle: invalid DH field type")
		}
	}
	gen := new(big.Int).SetBytes(dh[2].data)
	prime := new(big.Int).SetBytes(dh[3].data)
	pub := new(big.Int).SetBytes(dh[4].data)
	one := big.NewInt(1)
	upper := new(big.Int).Sub(prime, one)
	if gen.Cmp(one) <= 0 || gen.Cmp(upper) >= 0 || pub.Cmp(one) <= 0 || pub.Cmp(upper) >= 0 {
		return errors.New("oracle: invalid DH group")
	}
	privateBytes := make([]byte, n)
	if _, e = rand.Read(privateBytes); e != nil {
		return e
	}
	private := new(big.Int).SetBytes(privateBytes)
	public := make([]byte, n)
	new(big.Int).Exp(gen, private, prime).FillBytes(public)
	key := make([]byte, n)
	new(big.Int).Exp(pub, private, prime).FillBytes(key)
	iv := dh[5].data
	var cipherIV []byte
	if encNew || hashNew {
		if len(key) < 64 {
			return errors.New("oracle: short modern DH key")
		}
		derivedIV := bytes.Clone(key[32:64])
		if encNew {
			cipherIV = derivedIV[:16]
		}
		if hashNew {
			iv = derivedIV
		}
	}
	if e = s.sendANO(anoService(3, anoField{1, public})); e != nil {
		return e
	}
	switch encID {
	case 0:
	case 1, 6, 8, 10:
		sizes := map[int]int{1: 40, 6: 256, 8: 56, 10: 128}
		s.crypt, e = NewOracleNetworkRC4Cryptor(key, dh[5].data, sizes[encID])
	case 15, 16, 17:
		s.crypt, e = NewOracleNetworkCBCEncrypter(key[:16+(encID-15)*8], cipherIV)
	default:
		return fmt.Errorf("oracle: unsupported network cipher %d", encID)
	}
	if e != nil {
		return e
	}
	switch hashID {
	case 0:
	case 1:
		s.integrity, e = NewOracleNetworkHash(md5.New(), key, iv, hashNew)
	case 3:
		s.integrity, e = NewOracleNetworkHash(sha1.New(), key, iv, hashNew)
	case 4, 5, 6:
		if len(iv) < 16 {
			return errors.New("oracle: missing integrity IV")
		}
		switch hashID {
		case 4:
			s.integrity, e = NewOracleNetworkHash2(sha512.New(), key, iv, hashNew)
		case 5:
			s.integrity, e = NewOracleNetworkHash2(sha256.New(), key, iv, hashNew)
		case 6:
			s.integrity, e = NewOracleNetworkHash2(sha512.New384(), key, iv, hashNew)
		}
	default:
		return fmt.Errorf("oracle: unsupported integrity algorithm %d", hashID)
	}
	return e
}
