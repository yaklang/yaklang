// Derived from github.com/sijms/go-ora v3.0.1 (025c515), copyright 2020 Samy Sultan.
// Narrowed to password login only; see LICENSE.
package oracleprobe

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type AuthObject struct {
	EServerSessKey  string
	EClientSessKey  string
	EPassword       string
	ESpeedyKey      string
	ServerSessKey   []byte
	ClientSessKey   []byte
	KeyHash         []byte
	Salt            string
	pbkdf2ChkSalt   string
	pbkdf2VgenCount int
	pbkdf2SderCount int
	VerifierType    int
	tcpNego         *TCPNego
	conn            *Connection
}

// The 10g verifier depends on legacy database character mappings. Do not
// guess those mappings after removing the full driver's conversion tables.
var ErrUnsupportedCredentialEncoding = errors.New("oracle: legacy verifier requires ASCII credentials")

func asciiCredential(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func (obj *AuthObject) read() error {
	loop := true
	session := obj.conn.session
	for loop {
		messageCode, err := session.GetByte()
		if err != nil {
			return err
		}
		switch messageCode {
		case 8:
			dictLen, err := session.count(256)
			if err != nil {
				return err
			}
			for x := 0; x < dictLen; x++ {
				key, val, num, err := session.GetKeyVal()
				if err != nil {
					return err
				}
				if bytes.Equal(key, []byte("AUTH_SESSKEY")) {
					if len(obj.EServerSessKey) == 0 {
						obj.EServerSessKey = string(val)
					}
				} else if bytes.Equal(key, []byte("AUTH_VFR_DATA")) {
					if len(obj.Salt) == 0 {
						obj.Salt = string(val)
						obj.VerifierType = num
					}
				} else if bytes.Equal(key, []byte("AUTH_PBKDF2_CSK_SALT")) {
					if len(obj.pbkdf2ChkSalt) == 0 {
						obj.pbkdf2ChkSalt = string(val)
						if len(obj.pbkdf2ChkSalt) != 32 {
							return &Error{Code: 28041}
						}
					}
				} else if bytes.Equal(key, []byte("AUTH_PBKDF2_VGEN_COUNT")) {
					if obj.pbkdf2VgenCount == 0 {
						obj.pbkdf2VgenCount, err = strconv.Atoi(string(val))
						if err != nil {
							return &Error{Code: 28041}
						}
						if obj.pbkdf2VgenCount < 4096 || obj.pbkdf2VgenCount > 1000000 {
							return errors.New("oracle: invalid PBKDF2 verifier count")
						}
					}
				} else if bytes.Equal(key, []byte("AUTH_PBKDF2_SDER_COUNT")) {
					obj.pbkdf2SderCount, err = strconv.Atoi(string(val))
					if err != nil || obj.pbkdf2SderCount < 3 || obj.pbkdf2SderCount > 1000000 {
						return errors.New("oracle: invalid PBKDF2 derivation count")
					}
				}
			}
		default:
			err = obj.conn.ProcessTCCResponse(messageCode)
			if err != nil {
				return err
			}
			if messageCode == 4 {
				if session.HasError() {
					return session.GetError()
				}
				loop = false
			}
		}
	}
	return obj.deriveResponse()
}

func (obj *AuthObject) deriveResponse() error {

	if len(obj.EServerSessKey) != 64 && len(obj.EServerSessKey) != 96 {
		return errors.New("session key should be either 64, 96 bytes long")
	}
	var key []byte
	var speedyKey []byte
	padding := false
	var err error

	if obj.VerifierType == 2361 {
		if !asciiCredential(obj.conn.connOption.UserID) || !asciiCredential(obj.conn.connOption.Password) {
			return ErrUnsupportedCredentialEncoding
		}
		key, err = getKeyFromUserNameAndPassword(obj.conn.connOption.UserID, obj.conn.connOption.Password)
		if err != nil {
			return err
		}
	} else if obj.VerifierType == 6949 {

		if obj.tcpNego.ServerCompileTimeCaps[4]&2 == 0 {
			padding = true
		}
		result, err := hex.DecodeString(obj.Salt)
		if err != nil {
			return err
		}
		result = append([]byte(obj.conn.connOption.Password), result...)
		hash := sha1.New()
		_, err = hash.Write(result)
		if err != nil {
			return err
		}
		key = hash.Sum(nil)           // 20 byte key
		key = append(key, 0, 0, 0, 0) // 24 byte key
	} else if obj.VerifierType == 18453 {
		salt, err := hex.DecodeString(obj.Salt)
		if err != nil {
			return err
		}
		message := append(salt, []byte("AUTH_PBKDF2_SPEEDY_KEY")...)
		speedyKey, err = generateSpeedyKey(obj.conn.ctx, message, []byte(obj.conn.connOption.Password), obj.pbkdf2VgenCount)
		if err != nil {
			return err
		}

		buffer := append(speedyKey, salt...)
		hash := sha512.New()
		hash.Write(buffer)
		key = hash.Sum(nil)[:32]
	} else {
		return errors.New("unsupported verifier type")
	}
	obj.ServerSessKey, err = decryptSessionKey(padding, key, obj.EServerSessKey)
	if err != nil {
		return err
	}
	obj.ClientSessKey = make([]byte, len(obj.ServerSessKey))
	for {
		_, err = rand.Read(obj.ClientSessKey)
		if err != nil {
			return err
		}
		if !bytes.Equal(obj.ClientSessKey, obj.ServerSessKey) {
			break
		}
	}
	obj.EClientSessKey, err = encryptSessionKey(padding, key, obj.ClientSessKey)
	if err != nil {
		return err
	}
	newKey, err := obj.generatePasswordEncKey()
	if err != nil {
		return err
	}
	if obj.VerifierType == 18453 {
		padding = false
	} else {
		padding = true
	}
	obj.KeyHash = newKey
	obj.EPassword, err = encryptPassword([]byte(obj.conn.connOption.Password), newKey, true)
	if err != nil {
		return err
	}
	if obj.VerifierType == 18453 {
		obj.ESpeedyKey, err = encryptPassword(speedyKey, newKey, padding)
		if err != nil {
			return err
		}
	}
	return nil
}
func (obj *AuthObject) Write() error {
	session := obj.conn.session
	connOption := obj.conn.connOption
	mode := obj.conn.LogonMode
	keys := make([]string, 0, 20)
	values := make([]string, 0, 20)
	flags := make([]uint8, 0, 20)
	appendKeyVal := func(key, val string, f uint8) {
		keys = append(keys, key)
		values = append(values, val)
		flags = append(flags, f)
	}
	index := 0
	appendKeyVal("AUTH_CLIENT_CAPABILITIES", "1", 0)
	index++
	if len(obj.EClientSessKey) > 0 {
		appendKeyVal("AUTH_SESSKEY", obj.EClientSessKey, 1)
		index++
	}
	if len(obj.EPassword) > 0 {
		appendKeyVal("AUTH_PASSWORD", obj.EPassword, 0)
		index++
	}
	if len(obj.ESpeedyKey) > 0 {
		appendKeyVal("AUTH_PBKDF2_SPEEDY_KEY", obj.ESpeedyKey, 0)
		index++
	}
	appendKeyVal("AUTH_TERMINAL", connOption.ClientInfo.HostName, 0)
	index++
	appendKeyVal("AUTH_PROGRAM_NM", connOption.ClientInfo.ProgramName, 0)
	index++
	appendKeyVal("AUTH_MACHINE", connOption.ClientInfo.HostName, 0)
	index++
	appendKeyVal("AUTH_PID", fmt.Sprintf("%d", connOption.ClientInfo.PID), 0)
	index++
	if len(connOption.ClientInfo.OSUserName) > 0 {
		appendKeyVal("AUTH_SID", connOption.ClientInfo.OSUserName, 0)
		index++
	}
	appendKeyVal("AUTH_CONNECT_STRING", connOption.ConnectionData(), 0)
	index++
	appendKeyVal("SESSION_CLIENT_CHARSET", strconv.Itoa(int(obj.tcpNego.ServerCharset)), 0)
	index++
	appendKeyVal("SESSION_CLIENT_LIB_TYPE", "0", 0)
	index++
	appendKeyVal("SESSION_CLIENT_DRIVER_NAME", connOption.ClientInfo.DriverName, 0)
	index++
	appendKeyVal("SESSION_CLIENT_VERSION", "3.0.0.0", 0)
	index++
	appendKeyVal("SESSION_CLIENT_LOBATTR", "1", 0)
	index++
	session.PutTTCFunc(0x3, 0x73)
	if len(connOption.UserID) > 0 {
		session.PutBytes(1)
		session.PutInt(len(connOption.UserID), 4, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	if len(connOption.UserID) > 0 && len(obj.EPassword) > 0 {
		mode |= UserAndPass
	}
	session.PutUint(int(mode|NoNewPass), 4, true, true)
	session.PutBytes(1)
	session.PutUint(index, 4, true, true)
	session.PutBytes(1, 1)
	if len(connOption.UserID) > 0 {
		session.PutString(connOption.UserID)
	}
	for i := 0; i < index; i++ {
		session.PutKeyValString(keys[i], values[i], flags[i])
	}
	return session.Write()
}

func generateSpeedyKey(ctx context.Context, buffer, key []byte, turns int) ([]byte, error) {
	if turns < 1 || turns > 1000000 {
		return nil, fmt.Errorf("oracle: PBKDF2 count outside probe limit: %d", turns)
	}
	mac := hmac.New(sha512.New, key)
	mac.Write(append(buffer, 0, 0, 0, 1))
	firstHash := mac.Sum(nil)
	tempHash := make([]byte, len(firstHash))
	copy(tempHash, firstHash)
	for index1 := 2; index1 <= turns; index1++ {
		if index1%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		mac.Reset()
		mac.Write(tempHash)
		tempHash = mac.Sum(nil)
		for index2 := 0; index2 < 64; index2++ {
			firstHash[index2] = firstHash[index2] ^ tempHash[index2]
		}
	}
	return firstHash, ctx.Err()
}

func getKeyFromUserNameAndPassword(username string, password string) ([]byte, error) {
	username = strings.ToUpper(username)
	password = strings.ToUpper(password)
	extendString := func(str string) []byte {
		ret := make([]byte, len(str)*2)
		for index, char := range []byte(str) {
			ret[index*2] = 0
			ret[index*2+1] = char
		}
		return ret
	}
	buffer := append(extendString(username), extendString(password)...)
	if len(buffer)%8 > 0 {
		buffer = append(buffer, make([]byte, 8-len(buffer)%8)...)
	}
	key := []byte{1, 35, 69, 103, 137, 171, 205, 239}

	DesEnc := func(input []byte, key []byte) ([]byte, error) {
		ret := make([]byte, 8)
		enc, err := des.NewCipher(key)
		if err != nil {
			return nil, err
		}
		for x := 0; x < len(input)/8; x++ {
			for y := 0; y < 8; y++ {
				ret[y] = uint8(int(ret[y]) ^ int(input[x*8+y]))
			}
			output := make([]byte, 8)
			enc.Encrypt(output, ret)
			copy(ret, output)
		}
		return ret, nil
	}
	key1, err := DesEnc(buffer, key)
	if err != nil {
		return nil, err
	}
	key2, err := DesEnc(buffer, key1)
	if err != nil {
		return nil, err
	}
	return append(key2, make([]byte, 8)...), nil
}
func decryptSessionKey(padding bool, encKey []byte, sessionKey string) ([]byte, error) {
	result, err := hex.DecodeString(sessionKey)
	if err != nil {
		return nil, err
	}
	if len(result) == 0 || len(result)%aes.BlockSize != 0 {
		return nil, errors.New("oracle: malformed encrypted session key")
	}
	blk, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	enc := cipher.NewCBCDecrypter(blk, make([]byte, 16))
	output := make([]byte, len(result))
	enc.CryptBlocks(output, result)
	cutLen := 0
	if padding {
		num := int(output[len(output)-1])
		if num > 0 && num <= enc.BlockSize() {
			apply := true
			for x := len(output) - num; x < len(output); x++ {
				if output[x] != uint8(num) {
					apply = false
					break
				}
			}
			if apply {
				cutLen = int(output[len(output)-1])
			}
		}
	}
	return output[:len(output)-cutLen], nil
}
func encryptSessionKey(padding bool, encKey []byte, sessionKey []byte) (string, error) {
	blk, err := aes.NewCipher(encKey)
	if err != nil {
		return "", err
	}
	enc := cipher.NewCBCEncrypter(blk, make([]byte, 16))
	originalLen := len(sessionKey)
	sessionKey = PKCS5Padding(sessionKey, blk.BlockSize())
	output := make([]byte, len(sessionKey))
	enc.CryptBlocks(output, sessionKey)
	if !padding {
		return fmt.Sprintf("%X", output[:originalLen]), nil
	}
	return fmt.Sprintf("%X", output), nil
}
func encryptPassword(password, key []byte, padding bool) (string, error) {
	buff1 := make([]byte, 0x10)
	_, err := rand.Read(buff1)
	if err != nil {
		return "", err
	}
	buffer := append(buff1, password...)
	return encryptSessionKey(padding, key, buffer)
}
func (obj *AuthObject) generatePasswordEncKey() ([]byte, error) {
	hash := md5.New()
	key1 := obj.ServerSessKey
	key2 := obj.ClientSessKey
	if len(key1) < 32 || len(key1) != len(key2) {
		return nil, errors.New("oracle: invalid session key size")
	}
	start := 16

	logonCompatibility := obj.tcpNego.ServerCompileTimeCaps[4]
	if logonCompatibility&32 != 0 {
		var keyBuffer string
		var retKeyLen int
		switch obj.VerifierType {
		case 2361:
			buffer := append(key2[:len(key2)/2], key1[:len(key1)/2]...)
			keyBuffer = fmt.Sprintf("%X", buffer)
			retKeyLen = 16
		case 6949:
			buffer := append(key2[:24], key1[:24]...)
			keyBuffer = fmt.Sprintf("%X", buffer)
			retKeyLen = 24
		case 18453:
			buffer := append(key2, key1...)
			keyBuffer = fmt.Sprintf("%X", buffer)
			retKeyLen = 32
		default:
			return nil, errors.New("unsupported verifier type")
		}
		df2key, err := hex.DecodeString(obj.pbkdf2ChkSalt)
		if err != nil {
			return nil, err
		}
		derived, err := generateSpeedyKey(obj.conn.ctx, df2key, []byte(keyBuffer), obj.pbkdf2SderCount)
		if err != nil {
			return nil, err
		}
		return derived[:retKeyLen], nil
	} else {
		switch obj.VerifierType {
		case 2361:
			buffer := make([]byte, 16)
			for x := 0; x < 16; x++ {
				buffer[x] = key1[x+start] ^ key2[x+start]
			}
			_, err := hash.Write(buffer)
			if err != nil {
				return nil, err
			}
			return hash.Sum(nil), nil
		case 6949:
			// Only the legacy XOR derivation reads bytes 16..39. Oracle 12c
			// can send a 32-byte nonce with the modern PBKDF2 derivation.
			if len(key1) < 40 {
				return nil, errors.New("oracle: truncated legacy session key")
			}
			buffer := make([]byte, 24)
			for x := 0; x < 24; x++ {
				buffer[x] = key1[x+start] ^ key2[x+start]
			}
			_, err := hash.Write(buffer[:16])
			if err != nil {
				return nil, err
			}
			ret := hash.Sum(nil)
			hash.Reset()
			_, err = hash.Write(buffer[16:])
			if err != nil {
				return nil, err
			}
			ret = append(ret, hash.Sum(nil)...)
			return ret[:24], nil
		default:
			return nil, errors.New("unsupported verifier type")
		}
	}
}
