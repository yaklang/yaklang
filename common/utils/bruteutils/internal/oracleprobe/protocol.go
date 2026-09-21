// Derived from github.com/sijms/go-ora v3.0.1 (025c515), copyright 2020 Samy Sultan.
// Narrowed to password login only; see LICENSE.
package oracleprobe

import (
	"encoding/binary"
	"errors"
	"fmt"
)

type TCPNego struct {
	conn                  *Connection
	MessageCode           uint8
	ProtocolServerString  string
	OracleVersion         int
	ServerCharset         int
	ServerFlags           uint8
	ServerNCharset        int
	ServerCompileTimeCaps []byte
	ServerRuntimeCaps     []byte
}

func (nego *TCPNego) writeMessage() {
	session := nego.conn.session
	session.PutBytes(1, 6, 0)
	session.PutBytes([]byte("OracleClientGo\x00")...)
}

func (nego *TCPNego) write() error {
	session := nego.conn.session
	session.ResetBuffer()
	nego.writeMessage()
	return session.Write()
}

func (nego *TCPNego) readMessage() error {
	var err error
	session := nego.conn.session
	nego.MessageCode, err = session.GetByte()
	if err != nil {
		return err
	}
	if nego.MessageCode != 1 {
		return fmt.Errorf("message code error: received code %d and expected code is 1", nego.MessageCode)
	}
	var protocolServerVersion uint8
	protocolServerVersion, err = session.GetByte()
	if err != nil {
		return err
	}
	switch protocolServerVersion {
	case 4:
		nego.OracleVersion = 7230
	case 5:
		nego.OracleVersion = 8030
	case 6:
		nego.OracleVersion = 8100
	default:
		return fmt.Errorf("unsupported protocol server version: %d", protocolServerVersion)
	}
	_, err = session.GetByte()
	if err != nil {
		return err
	}
	nego.ProtocolServerString, err = session.GetNullTermString()
	if err != nil {
		return err
	}
	nego.ServerCharset, err = session.GetInt(2, false, false)
	if err != nil {
		return err
	}
	nego.ServerFlags, err = session.GetByte()
	if err != nil {
		return err
	}
	var charsetElem int
	charsetElem, err = session.GetInt(2, false, false)
	if err != nil {
		return err
	}
	if charsetElem > 0 {
		_, _ = session.GetBytes(charsetElem * 5)
	}
	len1, err := session.GetInt(2, false, true)
	if err != nil {
		return err
	}
	numArray, err := session.GetBytes(len1)
	if err != nil {
		return err
	}
	if len(numArray) < 7 {
		return errors.New("oracle: truncated charset negotiation")
	}
	num3 := 6 + int(numArray[5]) + int(numArray[6])
	if num3+5 > len(numArray) {
		return errors.New("oracle: invalid national charset offset")
	}
	nego.ServerNCharset = int(binary.BigEndian.Uint16(numArray[(num3 + 3):(num3 + 5)]))
	len2, err := session.GetByte()
	if err != nil {
		return err
	}
	nego.ServerCompileTimeCaps, err = session.GetBytes(int(len2))
	if err != nil {
		return err
	}
	len3, err := session.GetByte()
	if err != nil {
		return err
	}
	nego.ServerRuntimeCaps, err = session.GetBytes(int(len3))
	if err != nil {
		return err
	}
	if len(nego.ServerCompileTimeCaps) > 15 && nego.ServerCompileTimeCaps[15]&1 != 0 {
		session.HasEOSCapability = true
	}
	if len(nego.ServerCompileTimeCaps) > 16 && nego.ServerCompileTimeCaps[16]&1 != 0 {
		session.HasFSAPCapability = true
	}
	if nego.ServerCompileTimeCaps == nil || len(nego.ServerCompileTimeCaps) < 8 {
		return errors.New("server compile time caps length less than 8")
	}
	if len(nego.ServerCompileTimeCaps) > 37 && nego.ServerCompileTimeCaps[37]&32 != 0 {
		session.UseBigClrChunks = true
		session.ClrChunkSize = 0x7FFF
	}

	return nil
}
