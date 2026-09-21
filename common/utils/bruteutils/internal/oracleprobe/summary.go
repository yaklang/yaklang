// Derived from go-ora v3.0.1 network/summary_object.go; see LICENSE.
package oracleprobe

import "fmt"

type BindError struct {
	errorCode int
	rowOffset int
	errorMsg  []byte
}
type SummaryObject struct {
	EndOfCallStatus      int // uint32
	EndToEndECIDSequence int // uint16
	CurRowNumber         int // uint32
	RetCode              int // uint16
	arrayElmWError       int // uint16
	arrayElmErrno        int // uint16
	CursorID             int // uint16
	errorPos             int // uint16
	sqlType              uint8
	oerFatal             uint8
	Flags                int // uint16
	userCursorOPT        int // uint16
	upiParam             uint8
	warningFlag          uint8
	rba                  int // uint32
	partitionID          int // uint16
	tableID              uint8
	blockNumber          int // uint32
	slotNumber           int // uint16
	osError              int // uint32
	stmtNumber           uint8
	callNumber           uint8
	pad1                 int // uint16
	successIter          int // uint16
	ErrorMessage         []byte
	bindErrors           []BindError
}

func NewSummary(session *session) (*SummaryObject, error) {
	result := new(SummaryObject)
	var err error
	if session.HasEOSCapability {
		result.EndOfCallStatus, err = session.GetInt(4, true, true)
		if err != nil {
			return nil, err
		}
	}
	if session.TTCVersion >= 3 {
		if session.HasFSAPCapability {
			result.EndToEndECIDSequence, err = session.GetInt(2, true, true)
			if err != nil {
				return nil, err
			}

		}
	}
	result.CurRowNumber, err = session.GetInt(4, true, true)
	if err != nil {
		return nil, err
	}
	result.RetCode, err = session.GetInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.arrayElmWError, err = session.GetInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.arrayElmErrno, err = session.GetInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.CursorID, err = session.GetInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.errorPos, err = session.GetInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.sqlType, err = session.GetByte()
	if err != nil {
		return nil, err
	}
	result.oerFatal, err = session.GetByte()
	if err != nil {
		return nil, err
	}
	if session.TTCVersion >= 4 {
		result.Flags, err = session.GetInt(2, true, true)
		if err != nil {
			return nil, err
		}
		result.userCursorOPT, err = session.GetInt(2, true, true)
		if err != nil {
			return nil, err
		}
	} else {
		var temp uint8
		temp, err = session.GetByte()
		if err != nil {
			return nil, err
		}
		result.Flags = int(temp)
		temp, err = session.GetByte()
		if err != nil {
			return nil, err
		}
		result.userCursorOPT = int(temp)
	}
	result.upiParam, err = session.GetByte()
	if err != nil {
		return nil, err
	}
	result.warningFlag, err = session.GetByte()
	if err != nil {
		return nil, err
	}
	result.rba, err = session.GetInt(4, true, true)
	if err != nil {
		return nil, err
	}
	result.partitionID, err = session.GetInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.tableID, err = session.GetByte()
	if err != nil {
		return nil, err
	}
	result.blockNumber, err = session.GetInt(4, true, true)
	if err != nil {
		return nil, err
	}
	result.slotNumber, err = session.GetInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.osError, err = session.GetInt(4, true, true)
	if err != nil {
		return nil, err
	}
	result.stmtNumber, err = session.GetByte()
	if err != nil {
		return nil, err
	}
	result.callNumber, err = session.GetByte()
	if err != nil {
		return nil, err
	}
	result.pad1, err = session.GetInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.successIter, err = session.GetInt(4, true, true)
	if err != nil {
		return nil, err
	}
	_, _ = session.GetDlc()
	if session.TTCVersion < 7 {
		_, _ = session.GetDlc()
		_, _ = session.GetDlc()
		_, _ = session.GetDlc()
	} else {
		for _, width := range []int{2, 4, 2} {
			n, err := session.GetInt(width, true, true)
			if err != nil {
				return nil, err
			}
			if n != 0 {
				return nil, session.fail(fmt.Errorf("oracle: unexpected SQL bind errors during authentication"))
			}
		}

		if session.TTCVersion >= 7 {
			result.RetCode, err = session.GetInt(4, true, true)
			if err != nil {
				return nil, err
			}
			result.CurRowNumber, err = session.GetInt(8, true, true)
			if err != nil {
				return nil, err
			}
		}
	}
	if result.RetCode != 0 {
		result.ErrorMessage, err = session.GetClr()
		if err != nil {
			return nil, err
		}
	}
	if len(result.bindErrors) > 0 && result.RetCode == 24381 {
		result.RetCode = result.bindErrors[0].errorCode
		result.ErrorMessage = result.bindErrors[0].errorMsg
	}
	return result, session.err
}

type WarningObject struct {
	retCode      int
	flag         int
	errorMessage string
}

func NewWarningObject(session *session) (*WarningObject, error) {
	result := new(WarningObject)
	var err error
	result.retCode, err = session.GetInt(2, true, true)
	if err != nil {
		return nil, err
	}
	length, err := session.GetInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.flag, err = session.GetInt(2, true, true)
	if err != nil {
		return nil, err
	}
	if result.retCode == 0 || length == 0 {
		return nil, nil
	} else {
		msg, err := session.GetClr()
		if err != nil {
			return nil, err
		}
		result.errorMessage = string(msg)
	}
	return result, session.err
}
