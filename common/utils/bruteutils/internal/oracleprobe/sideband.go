// Derived from go-ora v3.0.1 connection.go login sideband handlers; see LICENSE.
package oracleprobe

import "fmt"

func (conn *Connection) loadNLSData() error {
	_, err := conn.session.GetInt(2, true, true)
	if err != nil {
		return err
	}
	_, err = conn.session.GetByte()
	if err != nil {
		return err
	}
	length, err := conn.session.count(256)
	if err != nil {
		return err
	}
	_, err = conn.session.GetByte()
	if err != nil {
		return err
	}
	for i := 0; i < length; i++ {
		nlsKey, nlsVal, nlsCode, err := conn.session.GetKeyVal()
		if err != nil {
			return err
		}
		_, _, _ = nlsKey, nlsVal, nlsCode
	}
	_, err = conn.session.GetInt(4, true, true)
	return err
}

func (conn *Connection) getServerNetworkInformation(code uint8) error {
	session := conn.session
	if code == 0 {
		_, err := session.GetByte()
		return err
	}
	switch code - 1 {
	case 1:
		length, err := session.count(256)
		if err != nil {
			return err
		}
		_, err = session.GetByte()
		if err != nil {
			return err
		}
		_, err = session.GetBytes(length)
		if err != nil {
			return err
		}
	case 3:
		_, err := session.GetInt(2, true, true)
		if err != nil {
			return err
		}
		_, err = session.GetByte()
		if err != nil {
			return err
		}
		length, err := session.count(256)
		if err != nil {
			return err
		}
		for i := 0; i < length; i++ {
			nlsKey, nlsVal, nlsCode, err := session.GetKeyVal()
			if err != nil {
				return err
			}
			_, _, _ = nlsKey, nlsVal, nlsCode
		}
		flag, err := session.GetInt(4, true, true)
		if err != nil {
			return err
		}
		sessionID, err := session.GetInt(4, true, true)
		if err != nil {
			return err
		}
		serialID, err := session.GetInt(2, true, true)
		if err != nil {
			return err
		}
		if flag&4 == 4 {
			_, _ = sessionID, serialID
		}
	case 4:
		err := conn.loadNLSData()
		if err != nil {
			return err
		}
	case 6:
		length, err := session.GetInt(4, true, true)
		if err != nil {
			return err
		}
		transactionID, err := session.GetClr()
		if err != nil {
			return err
		}
		if len(transactionID) != length {
			return fmt.Errorf("oracle: transaction ID length mismatch")
		}
	case 7:
		_, err := session.GetInt(2, true, true)
		if err != nil {
			return err
		}
		_, err = session.GetByte()
		if err != nil {
			return err
		}
		_, err = session.GetInt(4, true, true)
		if err != nil {
			return err
		}
		_, err = session.GetInt(4, true, true)
		if err != nil {
			return err
		}
		_, err = session.GetByte()
		if err != nil {
			return err
		}
		_, err = session.GetDlc()
		if err != nil {
			return err
		}
	case 8:
		_, err := session.GetInt(2, true, true)
		if err != nil {
			return err
		}
		_, err = session.GetByte()
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("oracle: unsupported login sideband %d", code)
	}
	return session.err
}
