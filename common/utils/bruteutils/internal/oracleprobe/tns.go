// Derived from go-ora v3.0.1 network packet/session code; see LICENSE.
package oracleprobe

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
)

const maxExchange = 1 << 20

type session struct {
	conn net.Conn
	// Close the underlying socket directly; TLS close_notify must not extend a probe.
	transport                           net.Conn
	ctx                                 context.Context
	tlsConfig                           *tls.Config
	tlsRestarts                         int
	in, out                             bytes.Buffer
	err                                 error
	version                             uint16
	sdu                                 int
	advanced                            bool
	encryption                          EncryptionPolicy
	encryptionName, integrityName       string
	received, packets                   int
	UseBigClrChunks                     bool
	ClrChunkSize                        int
	TTCVersion                          byte
	HasEOSCapability, HasFSAPCapability bool
	sequence                            byte
	crypt                               OracleNetworkEncryption
	integrity                           OracleNetworkDataIntegrity
	Summary                             *SummaryObject
}

// Oracle's TLS handoff starts a new TLS connection on the raw socket, not
// inside the old TLS stream. Every handshake retains the probe's deadline and
// certificate policy; the old wrapper must not send close_notify on this socket.
func (s *session) startTLS() error {
	if s.tlsRestarts >= 4 {
		return errors.New("oracle: TLS restart limit exceeded")
	}
	s.tlsRestarts++
	secured := tls.Client(s.transport, s.tlsConfig)
	if err := secured.HandshakeContext(s.ctx); err != nil {
		return err
	}
	s.conn = secured
	return nil
}

func (s *session) HasError() bool {
	return s.err != nil || (s.Summary != nil && s.Summary.RetCode != 0)
}
func (s *session) GetError() error {
	if s.err != nil {
		return s.err
	}
	if s.Summary != nil && s.Summary.RetCode != 0 {
		return &Error{Code: s.Summary.RetCode, Message: string(s.Summary.ErrorMessage)}
	}
	return nil
}
func (s *session) packet() ([]byte, error) {
	if s.packets >= 256 {
		return nil, errors.New("oracle: packet limit exceeded")
	}
	s.packets++
	var h [8]byte
	if _, e := io.ReadFull(s.conn, h[:]); e != nil {
		return nil, e
	}
	n := int(binary.BigEndian.Uint16(h[:]))
	if s.version >= 315 {
		n = int(binary.BigEndian.Uint32(h[:]))
	}
	if n < 8 || n > maxField || n > maxExchange-s.received {
		return nil, fmt.Errorf("oracle: invalid packet size %d", n)
	}
	s.received += n
	p := make([]byte, n)
	copy(p, h[:])
	_, e := io.ReadFull(s.conn, p[8:])
	return p, e
}
func (s *session) sendPacket(typ byte, p []byte) error {
	if len(p) > maxField-8 {
		return errors.New("oracle: outgoing packet too large")
	}
	b := make([]byte, 8, len(p)+8)
	b[4] = typ
	n := len(p) + 8
	if s.version >= 315 {
		binary.BigEndian.PutUint32(b, uint32(n))
	} else {
		binary.BigEndian.PutUint16(b, uint16(n))
	}
	b = append(b, p...)
	for len(b) > 0 {
		n, e := s.conn.Write(b)
		if e != nil {
			return e
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
func (s *session) writeData(b []byte) error {
	for len(b) > 0 {
		n := min(len(b), s.sdu-128)
		chunk := b[:n]
		b = b[n:]
		if s.integrity != nil {
			chunk = append(bytes.Clone(chunk), s.integrity.Compute(chunk)...)
		}
		var e error
		if s.crypt != nil {
			chunk, e = s.crypt.Encrypt(chunk)
			if e != nil {
				return e
			}
		}
		if s.crypt != nil || s.integrity != nil {
			chunk = append(chunk, 0)
		}
		if e = s.sendPacket(6, append([]byte{0, 0}, chunk...)); e != nil {
			return e
		}
	}
	return nil
}
func (s *session) data() ([]byte, error) {
	for {
		p, e := s.packet()
		if e != nil {
			return nil, e
		}
		switch p[4] {
		case 6:
			if len(p) < 10 {
				return nil, io.ErrUnexpectedEOF
			}
			if binary.BigEndian.Uint16(p[8:])&0x40 != 0 {
				return nil, io.EOF
			}
			if s.tlsConfig != nil && (p[5]&0x80 != 0 || binary.BigEndian.Uint16(p[8:])&0x8000 != 0) {
				if e := s.startTLS(); e != nil {
					return nil, e
				}
			}
			b := p[10:]
			if s.crypt != nil || s.integrity != nil {
				if len(b) < 2 {
					return nil, errors.New("oracle: truncated protected packet")
				}
				b = b[:len(b)-1]
			}
			if s.crypt != nil {
				b, e = s.crypt.Decrypt(b)
				if e != nil {
					return nil, e
				}
			}
			if s.integrity != nil {
				b, e = s.integrity.Validate(b)
				if e != nil {
					return nil, e
				}
			}
			if len(b) > 0 {
				return b, nil
			}
		case 12:

			if len(p) != 11 || p[8] != 1 || p[9] != 0 || p[10] < 1 || p[10] > 3 {
				return nil, errors.New("oracle: invalid marker")
			}
			if p[10] == 1 || p[10] == 3 {
				if e = s.sendPacket(12, []byte{1, 0, 2}); e != nil {
					return nil, e
				}
			}
			if p[10] == 2 {
				if s.crypt != nil {
					if e = s.crypt.Reset(); e != nil {
						return nil, e
					}
				}
				if s.integrity != nil {
					if e = s.integrity.Init(); e != nil {
						return nil, e
					}
				}
			}
		default:
			return nil, fmt.Errorf("oracle: unexpected TNS packet %d", p[4])
		}
	}
}

var redirectHost = regexp.MustCompile(`(?i)\(\s*HOST\s*=\s*([\w.:%-]+)\s*\)`)
var redirectPort = regexp.MustCompile(`(?i)\(\s*PORT\s*=\s*([0-9]+)\s*\)`)
var refuseCode = regexp.MustCompile(`(?i)\(\s*(?:ERR|CODE)\s*=\s*([0-9]+)\s*\)`)

func connect(ctx context.Context, d Dialer, o Options, descriptor string, details *Details) (*session, error) {
	address := o.Address
	for redirects := 0; redirects <= 2; redirects++ {
		if details != nil {
			details.Transport = "unknown"
		}
		conn, e := d.DialContext(ctx, "tcp", address)
		if e != nil {
			return nil, e
		}
		s := &session{conn: conn, transport: conn, ctx: ctx, encryption: o.Encryption, sdu: 8192, ClrChunkSize: 64}
		stop := context.AfterFunc(ctx, func() { conn.Close() })
		next, e := func() (string, error) {
			if deadline, ok := ctx.Deadline(); ok {
				if e := conn.SetDeadline(deadline); e != nil {
					return "", e
				}
			}
			if o.TLS != nil {
				s.tlsConfig = o.TLS.Clone()
				if s.tlsConfig.ServerName == "" {
					s.tlsConfig.ServerName, _, _ = net.SplitHostPort(address)
				}
				if e := s.startTLS(); e != nil {
					return "", e
				}
			}
			if details != nil {
				details.Transport = "tcp"
				if o.TLS != nil {
					details.Transport = "tcps"
				}
			}
			// v3.0.1 CONNECT layout. Fast-auth/pipelining are intentionally not advertised.
			b := make([]byte, 66)
			binary.BigEndian.PutUint16(b, 319)
			binary.BigEndian.PutUint16(b[2:], 300)
			binary.BigEndian.PutUint16(b[4:], 1|2048)
			binary.BigEndian.PutUint16(b[6:], 8192)
			binary.BigEndian.PutUint16(b[8:], 8192)
			b[10], b[11] = 79, 152
			binary.BigEndian.PutUint16(b[14:], 1)
			binary.BigEndian.PutUint16(b[16:], uint16(len(descriptor)))
			binary.BigEndian.PutUint16(b[18:], 74)
			b[24], b[25] = 1, 1
			binary.BigEndian.PutUint32(b[50:], 8192)
			binary.BigEndian.PutUint32(b[54:], 8192)
			if len(descriptor) <= 230 {
				b = append(b, descriptor...)
			}
			for resends := 0; resends < 3; resends++ {
				if e := s.sendPacket(1, b); e != nil {
					return "", e
				}
				if len(descriptor) > 230 {
					if e := s.writeData([]byte(descriptor)); e != nil {
						return "", e
					}
				}
				p, e := s.packet()
				if e != nil {
					return "", e
				}
				switch p[4] {
				case 11:
					// Native Oracle TCPS may hand off to a dedicated server and
					// request a fresh TLS session over the same TCP socket.
					if p[5]&8 != 0 && s.tlsConfig != nil {
						if e := s.startTLS(); e != nil {
							return "", e
						}
					}
					continue
				case 2:
					if len(p) < 32 {
						return "", errors.New("oracle: truncated ACCEPT")
					}
					v := binary.BigEndian.Uint16(p[8:])
					if v < 300 || v > 319 {
						return "", fmt.Errorf("oracle: unsupported TNS version %d", v)
					}
					n := int(binary.BigEndian.Uint16(p[12:]))
					if v >= 315 {
						if len(p) < 40 {
							return "", errors.New("oracle: truncated extended ACCEPT")
						}
						n = int(binary.BigEndian.Uint32(p[32:]))
					}
					if n < 512 || n > maxField {
						return "", fmt.Errorf("oracle: invalid negotiated SDU %d", n)
					}
					s.sdu = min(n, 8192)
					s.version = v
					s.advanced = p[22]&1 != 0 && p[22]&4 == 0 && p[23]&8 == 0
					return "", nil
				case 4:
					if len(p) < 12 {
						return "", errors.New("oracle: truncated REFUSE")
					}
					n := int(binary.BigEndian.Uint16(p[10:]))
					if n > len(p)-12 {
						return "", errors.New("oracle: invalid REFUSE length")
					}
					msg := string(p[12 : 12+n])
					code := 12500
					for _, m := range refuseCode.FindAllStringSubmatch(msg, -1) {
						v, _ := strconv.Atoi(m[1])
						if v != 0 {
							code = v
							break
						}
					}
					return "", &Error{Code: code, Message: "listener refused connection"}
				case 5:
					if len(p) < 10 {
						return "", errors.New("oracle: truncated REDIRECT")
					}
					n := int(binary.BigEndian.Uint16(p[8:]))
					data := p[10:]
					if len(data) == 0 {
						data, e = s.data()
						if e != nil {
							return "", e
						}
					}
					if n > len(data) {
						return "", errors.New("oracle: invalid redirect length")
					}
					text := string(data[:n])
					text = strings.SplitN(text, "\x00", 2)[0]
					if strings.Contains(strings.ToUpper(text), "TCPS") && o.TLS == nil {
						return "", ErrTLSRequired
					}
					h, p := redirectHost.FindStringSubmatch(text), redirectPort.FindStringSubmatch(text)
					if len(h) != 2 || len(p) != 2 {
						return "", errors.New("oracle: invalid redirect address")
					}
					port, e := strconv.Atoi(p[1])
					if e != nil || port < 1 || port > 65535 {
						return "", errors.New("oracle: invalid redirect port")
					}
					return net.JoinHostPort(h[1], p[1]), nil
				default:
					return "", fmt.Errorf("oracle: unexpected connect response %d", p[4])
				}
			}
			return "", errors.New("oracle: resend limit exceeded")
		}()
		stop()
		if e != nil {
			conn.Close()
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, e
		}
		if next == "" {
			return s, nil
		}
		conn.Close()
		address = next
	}
	return nil, errors.New("oracle: redirect limit exceeded")
}
