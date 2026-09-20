// Derived from go-ora v3.0.1 connection.doAuth and ProcessTCCResponse; see LICENSE.
package oracleprobe

import (
	"bytes"
	"errors"
	"fmt"
)

func (c *Connection) doAuth() error {
	if c.details != nil {
		c.details.Stage = "auth-challenge"
	}
	s := c.session
	s.ResetBuffer()
	s.PutTTCFunc(3, 0x76)
	s.PutBytes(1)
	s.PutUint(len(c.connOption.UserID), 4, true, true)
	s.PutUint(int(c.LogonMode), 4, true, true)
	s.PutBytes(1, 1, 5, 1, 1)
	s.PutString(c.connOption.UserID)
	for _, kv := range [][2]string{{"AUTH_TERMINAL", "yak"}, {"AUTH_PROGRAM_NM", "yak-oracle-probe"}, {"AUTH_MACHINE", "yak"}, {"AUTH_PID", "0"}, {"AUTH_SID", "yak"}} {
		s.PutKeyValString(kv[0], kv[1], 0)
	}
	if e := s.Write(); e != nil {
		return e
	}
	auth := &AuthObject{conn: c, tcpNego: c.tcpNego}
	e := auth.read()
	if c.details != nil {
		c.details.Verifier = auth.VerifierType
	}
	if e != nil {
		return e
	}
	if c.details != nil {
		c.details.Stage = "auth-response"
	}
	s.ResetBuffer()
	if e := auth.Write(); e != nil {
		return e
	}
	if c.details != nil {
		c.details.Stage = "auth-result"
	}
	e = c.readAuthResult(auth)
	if e == nil && c.details != nil {
		c.details.Stage = "complete"
	}
	return e
}

func (c *Connection) readAuthResult(auth *AuthObject) error {
	s := c.session
	c.SessionProperties = make(map[string]string)
	for {
		msg, e := s.GetByte()
		if e != nil {
			return e
		}
		if msg == 8 {
			n, e := s.count(256)
			if e != nil {
				return e
			}
			for i := 0; i < n; i++ {
				k, v, _, e := s.GetKeyVal()
				if e != nil {
					return e
				}
				if _, duplicate := c.SessionProperties[string(k)]; duplicate || len(c.SessionProperties) >= 256 {
					return errors.New("oracle: duplicate or excessive authentication properties")
				}
				c.SessionProperties[string(k)] = string(v)
			}
			continue
		}
		if e := c.ProcessTCCResponse(msg); e != nil {
			return e
		}
		if msg == 4 || msg == 9 {
			break
		}
	}
	if s.err != nil {
		return s.err
	}
	if s.in.Len() != 0 {
		return errors.New("oracle: trailing data after authentication completion")
	}
	// Authentication completion alone is insufficient: require the authenticated
	// session properties and the encrypted server proof (10.2 and later).
	if c.SessionProperties["AUTH_SESSION_ID"] == "" || c.SessionProperties["AUTH_SERIAL_NUM"] == "" {
		return errors.New("oracle: authentication completed without session identity")
	}
	proof := c.SessionProperties["AUTH_SVR_RESPONSE"]
	if proof == "" {
		return errors.New("oracle: missing server authentication proof")
	}
	clear, e := decryptSessionKey(true, auth.KeyHash, proof)
	if e != nil {
		return e
	}
	if len(clear) != 32 || !bytes.Equal(clear[16:32], []byte("SERVER_TO_CLIENT")) {
		return errors.New("oracle: invalid server authentication proof")
	}
	return nil
}
func (c *Connection) ProcessTCCResponse(code byte) error {
	s := c.session
	switch code {
	case 4:
		var e error
		s.Summary, e = NewSummary(s)
		if e != nil {
			return e
		}
		return s.GetError()
	case 9:
		if s.HasEOSCapability {
			if _, e := s.GetInt(4, true, true); e != nil {
				return e
			}
		}
		if s.HasFSAPCapability {
			if _, e := s.GetInt(2, true, true); e != nil {
				return e
			}
		}
	case 15:
		_, e := NewWarningObject(s)
		if e != nil {
			return e
		}
	case 23:
		n, e := s.GetByte()
		if e != nil {
			return e
		}
		return c.getServerNetworkInformation(n)
	default:
		return fmt.Errorf("oracle: unexpected TTC login message %d", code)
	}
	return s.err
}
