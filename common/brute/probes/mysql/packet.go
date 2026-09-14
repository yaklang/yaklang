// Package mysql 实现最小化的 MySQL/MariaDB 认证探针。
//
// 只包含认证探测需要的协议子集：Initial Handshake 解析、
// HandshakeResponse41 构造、mysql_native_password 与
// caching_sha2_password（快路径 + RSA 全量认证）、AuthSwitch 处理、
// OK/ERR 分类。不引入驱动、连接池、查询与类型系统。
package mysql

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	maxPacketSize = 1<<24 - 1
	// 默认 collation：utf8mb4_general_ci，5.5+ 均支持，Unicode 凭证必需。
	collationUTF8MB4 = 45
)

// 客户端能力位。
const (
	clientLongPassword               uint32 = 1 << iota // 1
	clientFoundRows                                     // 2
	clientLongFlag                                      // 4
	clientConnectWithDB                                 // 8
	clientNoSchema                                      // 16
	clientCompress                                      // 32
	clientODBC                                          // 64
	clientLocalFiles                                    // 128
	clientIgnoreSpace                                   // 256
	clientProtocol41                                    // 512
	clientInteractive                                   // 1024
	clientSSL                                           // 2048
	clientIgnoreSigpipe                                 // 4096
	clientTransactions                                  // 8192
	clientReserved                                      // 16384
	clientSecureConn                                    // 32768
	clientMultiStatements                               // 1 << 16
	clientMultiResults                                  // 1 << 17
	clientPSMultiResults                                // 1 << 18
	clientPluginAuth                                    // 1 << 19
	clientConnectAttrs                                  // 1 << 20
	clientPluginAuthLenEncClientData                    // 1 << 21
	clientCanHandleExpiredPasswords                     // 1 << 22
	clientSessionTrack                                  // 1 << 23
	clientDeprecateEOF                                  // 1 << 24
)

// 服务端包类型标记。
const (
	iOK             = 0x00
	iAuthMoreData   = 0x01
	iLocalInFile    = 0xfb
	iEOF            = 0xfe
	iERR            = 0xff
	cachingFastAuth = 0x03
	cachingFullAuth = 0x04
	cachingReqKey   = 0x02
)

// greeting 是服务端 Initial Handshake Packet 的解析结果。
type greeting struct {
	ProtocolVersion byte
	ServerVersion   string
	AuthPluginData  []byte // 完整 nonce（native 取前 20 字节）
	AuthPluginName  string
	CapabilityFlags uint32
}

var (
	errMalformedPacket = errors.New("mysql: malformed packet")
	errPacketTooLarge  = errors.New("mysql: packet too large")
)

// packetConn 包装带序号管理的 MySQL 分组读写。
type packetConn struct {
	r        io.Reader
	w        io.Writer
	sequence byte
}

func newPacketConn(rw io.ReadWriter) *packetConn {
	return &packetConn{r: rw, w: rw}
}

// setStream 切换底层读写流（TLS 升级）但保留包序号：
// MySQL 的 sequence id 跨 TLS 握手连续，不得重置。
func (p *packetConn) setStream(rw io.ReadWriter) {
	p.r = rw
	p.w = rw
}

// readPacket 读取一个完整 MySQL 分组（自动拼接超过 16MB 的分片）。
func (p *packetConn) readPacket() ([]byte, error) {
	var payload []byte
	for {
		var header [4]byte
		if _, err := io.ReadFull(p.r, header[:]); err != nil {
			return nil, err
		}
		length := int(uint32(header[0]) | uint32(header[1])<<8 | uint32(header[2])<<16)
		seq := header[3]
		if seq != p.sequence {
			return nil, fmt.Errorf("%w: sequence mismatch (got %d want %d)", errMalformedPacket, seq, p.sequence)
		}
		p.sequence++
		if length == 0 && len(payload) > 0 {
			continue
		}
		chunk := make([]byte, length)
		if _, err := io.ReadFull(p.r, chunk); err != nil {
			return nil, err
		}
		payload = append(payload, chunk...)
		if length < maxPacketSize {
			return payload, nil
		}
		if len(payload) > 64<<20 {
			return nil, errPacketTooLarge
		}
	}
}

// writePacket 写出一个分组（此处分组必然 < 16MB，无需分片）。
func (p *packetConn) writePacket(payload []byte) error {
	if len(payload) >= maxPacketSize {
		return errPacketTooLarge
	}
	header := []byte{
		byte(len(payload)),
		byte(len(payload) >> 8),
		byte(len(payload) >> 16),
		p.sequence,
	}
	p.sequence++
	if _, err := p.w.Write(header); err != nil {
		return err
	}
	_, err := p.w.Write(payload)
	return err
}

// parseGreeting 解析 Initial Handshake Packet（协议版本 10）。
func parseGreeting(payload []byte) (*greeting, error) {
	if len(payload) < 2 {
		return nil, errMalformedPacket
	}
	g := &greeting{ProtocolVersion: payload[0]}
	if g.ProtocolVersion != 10 {
		// 协议版本 9（pre-4.1）已消亡，视为协议不匹配。
		return nil, fmt.Errorf("%w: unsupported protocol version %d", errMalformedPacket, g.ProtocolVersion)
	}
	pos := 1
	// server version (NUL-terminated)
	end := indexByte(payload[pos:], 0)
	if end < 0 {
		return nil, errMalformedPacket
	}
	g.ServerVersion = string(payload[pos : pos+end])
	pos += end + 1
	// thread id (4)
	if pos+4 > len(payload) {
		return nil, errMalformedPacket
	}
	pos += 4
	// auth-plugin-data-part-1 (8)
	if pos+8 > len(payload) {
		return nil, errMalformedPacket
	}
	authData := append([]byte{}, payload[pos:pos+8]...)
	pos += 8
	// filler (1)
	if pos+1 > len(payload) {
		return nil, errMalformedPacket
	}
	pos++
	// capability lower (2)
	if pos+2 > len(payload) {
		return nil, errMalformedPacket
	}
	caps := uint32(binary.LittleEndian.Uint16(payload[pos : pos+2]))
	pos += 2
	if caps&clientProtocol41 == 0 {
		return nil, fmt.Errorf("%w: server does not support protocol 4.1", errMalformedPacket)
	}
	g.CapabilityFlags = caps
	if pos >= len(payload) {
		return nil, errMalformedPacket
	}
	// charset (1)
	pos++
	if pos+2 > len(payload) {
		return nil, errMalformedPacket
	}
	// status flags (2)
	pos += 2
	// capability upper (2)
	if pos+2 > len(payload) {
		return nil, errMalformedPacket
	}
	caps |= uint32(binary.LittleEndian.Uint16(payload[pos:pos+2])) << 16
	g.CapabilityFlags = caps
	pos += 2
	if caps&clientSecureConn == 0 {
		return nil, fmt.Errorf("%w: server does not support secure connection (4.1 pre-auth)", errMalformedPacket)
	}
	// auth plugin data len (1)
	if pos+1 > len(payload) {
		return nil, errMalformedPacket
	}
	pluginDataLen := int(payload[pos])
	pos++
	// reserved (10)
	if pos+10 > len(payload) {
		return nil, errMalformedPacket
	}
	pos += 10
	if caps&clientSecureConn != 0 {
		// auth-plugin-data-part-2: max(13, pluginDataLen - 8)
		n := 13
		if pluginDataLen > 0 {
			if pluginDataLen-8 > n {
				n = pluginDataLen - 8
			}
		}
		if pos+n > len(payload) {
			return nil, errMalformedPacket
		}
		part2 := payload[pos : pos+n]
		pos += n
		// 去掉结尾的 NUL
		if len(part2) > 0 && part2[len(part2)-1] == 0 {
			part2 = part2[:len(part2)-1]
		}
		authData = append(authData, part2...)
	}
	g.AuthPluginData = authData
	if caps&clientPluginAuth != 0 && pos < len(payload) {
		end := indexByte(payload[pos:], 0)
		if end >= 0 {
			g.AuthPluginName = string(payload[pos : pos+end])
		} else {
			g.AuthPluginName = string(payload[pos:])
		}
	}
	return g, nil
}

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// buildHandshakeResponse 构造 HandshakeResponse41。
// 本探针的 authResp（native 20 字节 / caching_sha2 32 字节）恒小于 251，
// 使用 1 字节长度前缀即可。
func buildHandshakeResponse(caps uint32, username string, authResp []byte, pluginName string) []byte {
	buf := make([]byte, 0, 128+len(username)+len(authResp))
	buf = binary.LittleEndian.AppendUint32(buf, caps)
	buf = binary.LittleEndian.AppendUint32(buf, maxPacketSize) // max packet size
	buf = append(buf, collationUTF8MB4)                        // charset
	buf = append(buf, make([]byte, 23)...)                     // reserved
	buf = append(buf, username...)
	buf = append(buf, 0)
	buf = append(buf, byte(len(authResp)))
	buf = append(buf, authResp...)
	buf = append(buf, pluginName...)
	buf = append(buf, 0)
	return buf
}

// buildSSLRequest 构造 SSLRequest 分组（32 字节，不含用户名等）。
func buildSSLRequest(caps uint32) []byte {
	caps |= clientSSL
	buf := make([]byte, 0, 32)
	buf = binary.LittleEndian.AppendUint32(buf, caps)
	buf = binary.LittleEndian.AppendUint32(buf, maxPacketSize)
	buf = append(buf, collationUTF8MB4)
	buf = append(buf, make([]byte, 23)...)
	return buf
}

// serverError 是 ERR 包解析结果。
type serverError struct {
	Code     uint16
	SQLState string
	Message  string
}

func (e *serverError) Error() string {
	if e.SQLState != "" {
		return fmt.Sprintf("mysql error %d (%s): %s", e.Code, e.SQLState, e.Message)
	}
	return fmt.Sprintf("mysql error %d: %s", e.Code, e.Message)
}

// parseErrPacket 解析 ERR 包。
func parseErrPacket(payload []byte) (*serverError, error) {
	if len(payload) < 3 {
		return nil, errMalformedPacket
	}
	err := &serverError{Code: binary.LittleEndian.Uint16(payload[1:3])}
	rest := payload[3:]
	if len(rest) > 0 && rest[0] == '#' && len(rest) >= 6 {
		err.SQLState = string(rest[1:6])
		rest = rest[6:]
	}
	err.Message = string(rest)
	return err, nil
}

// ParseGreetingForFuzz 仅供模糊测试导出。
func ParseGreetingForFuzz(payload []byte) (*greeting, error) {
	return parseGreeting(payload)
}
