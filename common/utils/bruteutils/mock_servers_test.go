package bruteutils_test

// 本地 mock 服务器正反向测试：为各协议提供最小协议实现的服务端，
// 验证 BrutePass 判定链路（正确凭证→Ok，错误凭证→!Ok，不可达→Finished）。
//
// 覆盖：FTP / SMTP(AUTH LOGIN) / Redis(RESP) / Memcached(stats) /
// Telnet(交互流) / LDAP(BER bindResponse) / VNC(RFB 3.8 VNC-Auth DES) /
// SNMPv2(UDP community)。
//
// 数据库协议（MySQL/PG/Mongo/MSSQL）的模拟器见 common/brute/probes/*，
// RDP/NLA 见 rdp_nla_test.go。

import (
	"bufio"
	"bytes"
	"crypto/des"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/bruteutils"
)

// ---------- 通用 ----------

func mockListen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

func mockProbe(t *testing.T, proto, target, user, pass string) *bruteutils.BruteItemResult {
	t.Helper()
	handler, err := bruteutils.GetBruteFuncByType(proto)
	if err != nil {
		t.Fatalf("no handler %s: %v", proto, err)
	}
	return handler(&bruteutils.BruteItem{
		Type:     proto,
		Target:   target,
		Username: user,
		Password: pass,
		Context:  nil,
	})
}

// 断言三元组：成功 / 认证失败（可继续） / 目标不可达（Finished）
func assertProbe(t *testing.T, name string, res *bruteutils.BruteItemResult, wantOK, wantFinished bool) {
	t.Helper()
	if res.Ok != wantOK {
		t.Errorf("[%s] ok=%v want %v", name, res.Ok, wantOK)
	}
	if res.Finished != wantFinished {
		t.Errorf("[%s] finished=%v want %v", name, res.Finished, wantFinished)
	}
}

func TestMockTargetUnavailable(t *testing.T) {
	// 全部协议对 127.0.0.1:1（立即拒绝）不得 panic；行为可分类。
	for _, proto := range []string{"ftp", "smtp", "redis", "telnet", "ldap", "vnc"} {
		t.Run(proto, func(t *testing.T) {
			res := mockProbe(t, proto, "127.0.0.1:1", "u", "p")
			if res.Ok {
				t.Errorf("[%s] unreachable target must not be ok", proto)
			}
		})
	}
}

// ---------- FTP ----------

func startMockFTP(t *testing.T, user, pass string) string {
	t.Helper()
	ln := mockListen(t)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				c.Write([]byte("220 mock ftp ready\r\n"))
				authed := false
				var gotUser string
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					cmd := strings.Fields(strings.TrimSpace(line))
					if len(cmd) == 0 {
						continue
					}
					switch strings.ToUpper(cmd[0]) {
					case "USER":
						gotUser = cmd[1]
						c.Write([]byte("331 password required\r\n"))
					case "PASS":
						if gotUser == user && cmd[1] == pass {
							authed = true
							c.Write([]byte("230 logged in\r\n"))
						} else {
							c.Write([]byte("530 login incorrect\r\n"))
						}
					case "QUIT":
						c.Write([]byte("221 bye\r\n"))
						return
					default:
						if authed {
							c.Write([]byte("200 ok\r\n"))
						} else {
							c.Write([]byte("530 not logged in\r\n"))
						}
					}
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func TestMockFTP(t *testing.T) {
	addr := startMockFTP(t, "admin", "FtpPass123!")
	assertProbe(t, "ftp-correct", mockProbe(t, "ftp", addr, "admin", "FtpPass123!"), true, false)
	assertProbe(t, "ftp-wrongpass", mockProbe(t, "ftp", addr, "admin", "WRONG"), false, false)
	assertProbe(t, "ftp-nouser", mockProbe(t, "ftp", addr, "nosuch", "FtpPass123!"), false, false)
}

// ---------- SMTP（AUTH LOGIN 全流程） ----------

func startMockSMTP(t *testing.T, user, pass string) string {
	t.Helper()
	ln := mockListen(t)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				w := c
				fmt.Fprint(w, "220 mock esmtp ready\r\n")
				readLine := func() string {
					line, err := r.ReadString('\n')
					if err != nil {
						return ""
					}
					return strings.TrimSpace(line)
				}
				var stage int // 0=user 1=pass
				for {
					line := readLine()
					if line == "" {
						return
					}
					switch {
					case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
						fmt.Fprint(w, "250-mock\r\n250-AUTH LOGIN\r\n250 OK\r\n")
					case strings.HasPrefix(line, "AUTH"):
						stage = 0
						fmt.Fprint(w, "334 "+base64.StdEncoding.EncodeToString([]byte("Username:"))+"\r\n")
					case strings.HasPrefix(line, "MAIL"), strings.HasPrefix(line, "RCPT"):
						fmt.Fprint(w, "250 ok\r\n")
					case strings.HasPrefix(line, "DATA"):
						fmt.Fprint(w, "354 go ahead\r\n")
					case strings.HasPrefix(line, "QUIT"):
						fmt.Fprint(w, "221 bye\r\n")
						return
					default:
						// base64 阶段或邮件体
						if stage <= 1 {
							decoded, err := base64.StdEncoding.DecodeString(line)
							if err != nil {
								fmt.Fprint(w, "501 bad base64\r\n")
								continue
							}
							if stage == 0 {
								if string(decoded) != user {
									fmt.Fprint(w, "535 auth failed\r\n")
									return
								}
								stage = 1
								fmt.Fprint(w, "334 "+base64.StdEncoding.EncodeToString([]byte("Password:"))+"\r\n")
							} else {
								if string(decoded) == pass {
									fmt.Fprint(w, "235 auth ok\r\n")
								} else {
									fmt.Fprint(w, "535 auth failed\r\n")
									return
								}
								stage = 2
							}
						} else {
							// 邮件体（含 . 结尾）
							if strings.TrimSpace(line) == "." {
								fmt.Fprint(w, "250 accepted\r\n")
							}
						}
					}
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func TestMockSMTP(t *testing.T) {
	addr := startMockSMTP(t, "alert@example.com", "SmtpPass123!")
	assertProbe(t, "smtp-correct", mockProbe(t, "smtp", addr, "alert@example.com", "SmtpPass123!"), true, false)
	assertProbe(t, "smtp-wrongpass", mockProbe(t, "smtp", addr, "alert@example.com", "WRONG"), false, true)
	assertProbe(t, "smtp-nouser", mockProbe(t, "smtp", addr, "nobody@example.com", "SmtpPass123!"), false, true)
}

// ---------- Redis（RESP） ----------

func startMockRedis(t *testing.T, pass string) string {
	t.Helper()
	ln := mockListen(t)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				kv := map[string]string{}
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					line = strings.TrimRight(line, "\r\n")
					if !strings.HasPrefix(line, "*") {
						continue
					}
					n, _ := strconv.Atoi(line[1:])
					parts := make([]string, 0, n)
					for i := 0; i < n; i++ {
						hl, err := r.ReadString('\n')
						if err != nil {
							return
						}
						l := strings.TrimRight(hl, "\r\n")
						if !strings.HasPrefix(l, "$") {
							continue
						}
						bl, _ := strconv.Atoi(l[1:])
						buf := make([]byte, bl+2)
						if _, err := readFull(r, buf); err != nil {
							return
						}
						parts = append(parts, string(buf[:bl]))
					}
					if len(parts) == 0 {
						continue
					}
					switch strings.ToUpper(parts[0]) {
					case "AUTH":
						if pass == "" || (len(parts) >= 2 && parts[1] == pass) {
							c.Write([]byte("+OK\r\n"))
						} else {
							c.Write([]byte("-ERR invalid password\r\n"))
							return
						}
					case "SET":
						if len(parts) >= 3 {
							kv[parts[1]] = parts[2]
							c.Write([]byte("+OK\r\n"))
						}
					case "GET":
						if v, ok := kv[parts[1]]; ok {
							fmt.Fprintf(c, "$%d\r\n%s\r\n", len(v), v)
						} else {
							c.Write([]byte("$-1\r\n"))
						}
					case "QUIT":
						c.Write([]byte("+OK\r\n"))
						return
					default:
						c.Write([]byte("+OK\r\n"))
					}
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func readFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func TestMockRedis(t *testing.T) {
	addr := startMockRedis(t, "RedisPass123!")
	// 正确密码：AUTH 过 + SET/GET 回环
	assertProbe(t, "redis-correct", mockProbe(t, "redis", addr, "", "RedisPass123!"), true, false)
	// 错误密码：AUTH 拒绝
	res := mockProbe(t, "redis", addr, "", "WRONG")
	if res.Ok {
		t.Errorf("[redis-wrongpass] must not be ok")
	}
	// 无密码模式（未授权）
	addr2 := startMockRedis(t, "")
	res2 := mockProbe(t, "redis", addr2, "", "")
	if !res2.Ok {
		t.Errorf("[redis-unauth] empty-auth server should allow: ok=%v", res2.Ok)
	}
}

// ---------- Memcached（stats 未授权路径） ----------

func TestMockMemcached(t *testing.T) {
	ln := mockListen(t)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				n, _ := c.Read(buf)
				if bytes.Contains(buf[:n], []byte("stats")) {
					c.Write([]byte("STAT pid 1\r\nSTAT version mock\r\nEND\r\n"))
				}
			}(conn)
		}
	}()
	res := mockProbe(t, "memcached", ln.Addr().String(), "", "")
	if !res.Ok {
		t.Errorf("[memcached-unauth] stats server should be unauth-ok")
	}
}

// ---------- Telnet（交互登录流） ----------

func startMockTelnet(t *testing.T, user, pass string) string {
	t.Helper()
	ln := mockListen(t)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				c.Write([]byte("mock login: "))
				u := readTelnetLine(r)
				c.Write([]byte("Password: "))
				p := readTelnetLine(r)
				if u == user && p == pass {
					c.Write([]byte("Login correct, welcome back\r\n"))
				} else {
					c.Write([]byte("Login incorrect\r\n"))
				}
				// 立即关闭：客户端稳定读取在 EOF 时返回，
				// 否则会等满超时（实测 3 用例 87s → 亚秒级）。
				_ = c.Close()
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func readTelnetLine(r *bufio.Reader) string {
	var sb strings.Builder
	for {
		b, err := r.ReadByte()
		if err != nil {
			return strings.TrimSpace(sb.String())
		}
		if b == '\n' {
			// 行结束即返回；空行返回空串（真实设备把单独的回车
			// 当作"继续"按键，例如敲回车后才显示登录提示）。
			return strings.TrimSpace(sb.String())
		}
		if b == '\r' {
			continue
		}
		sb.WriteByte(b)
	}
}

func TestMockTelnet(t *testing.T) {
	addr := startMockTelnet(t, "cisco", "Cisco123$")
	assertProbe(t, "telnet-correct", mockProbe(t, "telnet", addr, "cisco", "Cisco123$"), true, false)
	assertProbe(t, "telnet-wrongpass", mockProbe(t, "telnet", addr, "cisco", "WRONG"), false, false)
	assertProbe(t, "telnet-nouser", mockProbe(t, "telnet", addr, "nosuch", "Cisco123$"), false, false)
}

// ---------- LDAP（BER bindResponse） ----------

// ldapBindResponse 构造最小 LDAPMessage(BIND response)。
// resultCode: 0=success, 49=invalidCredentials
func ldapBindResponse(msgID int, resultCode int) []byte {
	// BindResponse ::= [APPLICATION 1] SEQUENCE { resultCode, matchedDN, diagMsg }
	body := []byte{0x0a, 0x01, byte(resultCode), 0x04, 0x00, 0x04, 0x00}
	inner := append([]byte{0x02, 0x01, byte(msgID), 0x61, byte(len(body))}, body...)
	return append([]byte{0x30, byte(len(inner))}, inner...)
}

func startMockLDAP(t *testing.T, user, pass string) string {
	t.Helper()
	ln := mockListen(t)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if err != nil || n == 0 {
						return
					}
					req := buf[:n]
					// 简化：bind 请求中的明文凭证以 UTF-8 出现在 BER 内
					okUser := bytes.Contains(req, []byte(user))
					okPass := bytes.Contains(req, []byte(pass))
					// 未授权探测（空 DN 空密码）：全部允许则 success
					code := 49
					if (okUser && okPass) || (!okUser && !okPass && bytes.Contains(req, []byte{0x80, 0x00})) {
						code = 0
					}
					c.Write(ldapBindResponse(1, code))
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func TestMockLDAP(t *testing.T) {
	addr := startMockLDAP(t, "admin", "LdapPass123!")
	assertProbe(t, "ldap-correct", mockProbe(t, "ldap", addr, "admin", "LdapPass123!"), true, false)
	assertProbe(t, "ldap-wrongpass", mockProbe(t, "ldap", addr, "admin", "WRONG"), false, false)
	assertProbe(t, "ldap-nouser", mockProbe(t, "ldap", addr, "ghost", "LdapPass123!"), false, false)
}

// ---------- VNC（RFB 3.8 + VNC-Auth DES） ----------

// vncDESChallenge 按 VNC 认证规范计算响应：密码补齐 8 字节、逐位反转，
// DES-ECB 加密 challenge。用于 mock 服务端校验客户端 PasswordAuth。
func vncDESChallenge(password string, challenge []byte) []byte {
	key := make([]byte, 8)
	copy(key, password)
	for i := range key {
		var b byte
		for j := 0; j < 8; j++ {
			if key[i]&(1<<uint(j)) != 0 {
				b |= 1 << uint(7-j)
			}
		}
		key[i] = b
	}
	cipher, _ := des.NewCipher(key)
	out := make([]byte, 16)
	cipher.Encrypt(out[:8], challenge[:8])
	cipher.Encrypt(out[8:], challenge[8:])
	return out
}

func startMockVNC(t *testing.T, pass string) string {
	t.Helper()
	ln := mockListen(t)
	var conns int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				id := atomic.AddInt32(&conns, 1)
				// RFB 握手
				c.Write([]byte("RFB 003.008\n"))
				ver := make([]byte, 12)
				if _, err := readFullN(c, ver); err != nil {
					return
				}
				// security types: [2]=VNC Authentication
				c.Write([]byte{1, 2})
				sel := make([]byte, 1)
				if _, err := readFullN(c, sel); err != nil {
					return
				}
				if sel[0] != 2 {
					return // 客户端不选 VNCAuth
				}
				challenge := make([]byte, 16)
				for i := 0; i < 16; i++ {
					challenge[i] = byte(i + int(id)) // 每连接不同
				}
				c.Write(challenge)
				resp := make([]byte, 16)
				if _, err := readFullN(c, resp); err != nil {
					return
				}
				var result uint32
				if bytes.Equal(resp, vncDESChallenge(pass, challenge)) {
					result = 0
				} else {
					result = 1
				}
				var rb [4]byte
				binary.BigEndian.PutUint32(rb[:], result)
				c.Write(rb[:])
				if result != 0 {
					_ = c.SetDeadline(time.Now().Add(time.Second))
					buf := make([]byte, 64)
					_, _ = c.Read(buf)
					return
				}
				// ClientInit / ServerInit
				ci := make([]byte, 1)
				if _, err := readFullN(c, ci); err != nil {
					return
				}
				var hdr [24]byte
				binary.BigEndian.PutUint16(hdr[0:2], 64)  // fb width
				binary.BigEndian.PutUint16(hdr[2:4], 48)  // fb height
				hdr[4] = 32                               // bits
				hdr[11] = 1                               // true color
				binary.BigEndian.PutUint16(hdr[12:14], 0) // red max? kept minimal
				var slen [4]byte
				binary.BigEndian.PutUint32(slen[:], 4)
				c.Write(hdr[:])
				c.Write(slen[:])
				c.Write([]byte("mock"))
				time.Sleep(200 * time.Millisecond)
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func readFullN(c net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := c.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func TestMockVNC(t *testing.T) {
	addr := startMockVNC(t, "VncPass123!")
	assertProbe(t, "vnc-correct", mockProbe(t, "vnc", addr, "", "VncPass123!"), true, false)
	assertProbe(t, "vnc-wrongpass", mockProbe(t, "vnc", addr, "", "WRONG"), false, false)
}

// ---------- SNMPv2（UDP community） ----------

func TestMockSNMPv2(t *testing.T) {
	// mock：community 正确的 GET(sysDescr.0) 返回完整合法的 GetResponse；
	// community 错误不响应（真实设备行为）→ 客户端超时失败。
	const community = "publ1c-mock"
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			req := append([]byte{}, buf[:n]...)
			if !bytes.Contains(req, []byte(community)) {
				continue // 错误 community：静默丢弃（真实设备行为）
			}
			resp := snmpGetResponse(req, community)
			if resp != nil {
				_, _ = pc.WriteTo(resp, addr)
			}
		}
	}()

	addr := pc.LocalAddr().String()
	res := mockProbe(t, "snmpv2", addr, "", community)
	if !res.Ok {
		t.Errorf("[snmpv2-correct] ok=%v finished=%v", res.Ok, res.Finished)
	}
	res2 := mockProbe(t, "snmpv2", addr, "", "wrong-community")
	if res2.Ok {
		t.Errorf("[snmpv2-wrong] must not be ok")
	}
}

// snmpGetResponse 从 GetRequest 报文提取 request-id，构造合法的
// GetResponse（sysDescr varbind）。BER 手工编码（最小实现）。
func snmpGetResponse(req []byte, community string) []byte {
	// 提取 request-id：SEQUENCE ver community [0] reqID ...
	// 简化扫描：在 community 之后找 INTEGER(02) 编码
	var reqID []byte
	if idx := bytes.Index(req, []byte(community)); idx >= 0 {
		rest := req[idx+len(community):]
		for i := 0; i+2 < len(rest); i++ {
			if rest[i] == 0x02 && rest[i+1] > 0 && rest[i+1] <= 4 {
				reqID = rest[i : i+2+int(rest[i+1])]
				break
			}
		}
	}
	if reqID == nil {
		return nil
	}
	// varbind: sysDescr(1.3.6.1.2.1.1.1.0) = "mock device"
	oid := []byte{0x2b, 0x06, 0x01, 0x02, 0x01, 0x01, 0x01, 0x00} // 1.3...
	val := []byte("mock device")
	vb := berTLV(0x30, append(berTLV(0x06, oid), berTLV(0x04, val)...))
	vbl := berTLV(0x30, vb)
	pdu := berTLV(0xa2, append(append(append(reqID,
		berTLV(0x02, []byte{0})...), berTLV(0x02, []byte{0})...), vbl...))
	msg := berTLV(0x30, append(append(
		berTLV(0x02, []byte{1}), // version 2c
		berTLV(0x04, []byte(community))...), pdu...))
	return msg
}

// berTLV 构造 BER TLV（长度短格式，足够 SNMP 小报文）。
func berTLV(tag byte, content []byte) []byte {
	out := []byte{tag, byte(len(content))}
	return append(out, content...)
}
