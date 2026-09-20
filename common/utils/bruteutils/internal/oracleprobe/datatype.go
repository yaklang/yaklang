// Derived from github.com/sijms/go-ora v3.0.1 (025c515), copyright 2020 Samy Sultan.
// Narrowed to password login only; see LICENSE.
package oracleprobe

import (
	"errors"
	"fmt"
	"time"
)

type DataTypeNego struct {
	conn                   *Connection
	MessageCode            uint8
	Server                 *TCPNego
	TypeAndRep             []uint16
	RuntimeTypeAndRep      []uint16
	DataTypeRepFor1100     uint16
	DataTypeRepFor1200     uint16
	CompileTimeCaps        []byte
	RuntimeCap             []byte
	b32kTypeSupported      bool
	supportSessionStateOps bool
	serverTZVersion        int
	clientTZVersion        int
}

func buildTypeNego(nego *TCPNego, conn *Connection) *DataTypeNego {
	result := DataTypeNego{
		conn:        conn,
		MessageCode: 2,
		Server:      nego,

		CompileTimeCaps: []byte{
			6, 1, 0, 0, 234, 28, 1, 24, 1, 1,
			1, 1, 1, 1, 0, 41, 144, 3, 7, 3,
			0, 1, 0, 235, 1, 63, 5, 1, 0, 0,
			0, 24, 0, 0, 10, 160, 2, 58, 0, 15,
			71, 0, 1, 0, 40, 0, 0, 0, 0, 0,
			0, 0, 2, 53,
		},

		RuntimeCap:             []byte{2, 1, 0, 0, 0, 0, 4 | 2},
		b32kTypeSupported:      false,
		supportSessionStateOps: false,
		clientTZVersion:        0x2B,
	}
	if result.Server != nil && len(result.Server.ServerCompileTimeCaps) > 0 {
		if len(result.Server.ServerCompileTimeCaps) <= 27 || result.Server.ServerCompileTimeCaps[27] == 0 {
			result.CompileTimeCaps[27] = 0
		}
		xmlTypeClientSideDecoding := false
		if len(result.Server.ServerCompileTimeCaps) > 7 {
			if result.Server.ServerCompileTimeCaps[7] >= 8 && xmlTypeClientSideDecoding {
				result.CompileTimeCaps[36] = 4
			} else if result.Server.ServerCompileTimeCaps[7] < 7 {
				result.CompileTimeCaps[36] = 0
			}
		}
		if len(result.Server.ServerRuntimeCaps) < 2 || result.Server.ServerRuntimeCaps[1]&1 != 1 {
			result.RuntimeCap[1] &= 0
		}
		if len(result.Server.ServerRuntimeCaps) > 6 {
			if result.Server.ServerRuntimeCaps[6]&4 == 4 {
				result.RuntimeCap[6] |= 4
				result.b32kTypeSupported = true
			}
			if result.Server.ServerRuntimeCaps[6]&16 == 16 {
				result.supportSessionStateOps = true
			}
			if result.Server.ServerRuntimeCaps[6]&2 == 2 {
				result.RuntimeCap[6] |= 2
			}
		}
		if len(result.Server.ServerCompileTimeCaps) <= 37 || result.Server.ServerCompileTimeCaps[37]&2 != 2 {
			result.CompileTimeCaps[37] = 0
			result.CompileTimeCaps[1] = 0
		}

	}
	result.TypeAndRep = typeRepresentations[:]
	result.DataTypeRepFor1100 = 1150
	result.DataTypeRepFor1200 = 1306

	if len(result.Server.ServerCompileTimeCaps) > 7 {
		if result.Server.ServerCompileTimeCaps[7] >= 8 {
			result.RuntimeTypeAndRep = result.TypeAndRep
		} else if result.Server.ServerCompileTimeCaps[7] >= 7 {
			result.RuntimeTypeAndRep = result.TypeAndRep[:result.DataTypeRepFor1200]
		} else {
			result.RuntimeTypeAndRep = result.TypeAndRep[:result.DataTypeRepFor1100]
		}
	} else {
		result.RuntimeTypeAndRep = result.TypeAndRep
	}
	return &result
}

func (nego *DataTypeNego) read() (zone *time.Location, err error) {
	var msg uint8
	session := nego.conn.session
	msg, err = session.GetByte()
	if err != nil {
		return
	}
	if msg != 2 {
		err = fmt.Errorf("message code error: received code %d and expected code is 2", msg)
		return
	}
	if nego.RuntimeCap[1] == 1 {
		var tz_bytes []byte
		tz_bytes, err = session.GetBytes(11)
		if err != nil {
			return
		}
		if len(tz_bytes) < 11 {
			err = errors.New("incorrect format for DBTimeZone")
			return
		}
		tzHours := int(tz_bytes[4]) - 60
		tzMin := int(tz_bytes[5]) - 60
		tzSec := int(tz_bytes[6]) - 60
		zone = time.FixedZone(fmt.Sprintf("%+03d:%02d", tzHours, tzMin),
			tzHours*60*60+tzMin*60+tzSec)
		if nego.CompileTimeCaps[37]&2 == 2 {
			nego.serverTZVersion, _ = session.GetInt(4, false, true)
		}
	}
	level := 0
	for {
		var num int
		if nego.CompileTimeCaps[27] == 0 {
			num, err = session.GetInt(1, false, false)
		} else {
			num, err = session.GetInt(2, false, true)
		}
		if err != nil {
			return nil, err
		}
		if num == 0 && level == 0 {
			break
		}
		if num == 0 && level == 1 {
			level = 0
			continue
		}
		if level == 3 {
			level = 0
			continue
		}
		level++
	}
	nego.conn.session.TTCVersion = min(nego.CompileTimeCaps[7], nego.Server.ServerCompileTimeCaps[7])
	return zone, session.err
}

func (nego *DataTypeNego) writeMessage() {
	if nego.Server.ServerCompileTimeCaps != nil && (len(nego.Server.ServerCompileTimeCaps) <= 27 || nego.Server.ServerCompileTimeCaps[27] == 0) {
		nego.CompileTimeCaps[27] = 0
	}
	session := nego.conn.session
	session.PutBytes(nego.MessageCode)
	session.PutInt(nego.Server.ServerCharset, 2, false, false)
	session.PutInt(nego.Server.ServerCharset, 2, false, false)
	session.PutBytes(nego.Server.ServerFlags, uint8(len(nego.CompileTimeCaps)))
	session.PutBytes(nego.CompileTimeCaps...)
	session.PutBytes(uint8(len(nego.RuntimeCap)))
	session.PutBytes(nego.RuntimeCap...)
	if nego.RuntimeCap[1]&1 == 1 {
		session.PutBytes(TZBytes()...)
		if nego.CompileTimeCaps[37]&2 == 2 {
			session.PutInt(nego.clientTZVersion, 4, true, false)
		}
	}
	session.PutInt(nego.Server.ServerNCharset, 2, false, false)
	if nego.CompileTimeCaps[27] == 0 {
		for _, x := range nego.RuntimeTypeAndRep {
			session.PutBytes(uint8(x))
		}
		session.PutBytes(0)
	} else {
		for _, x := range nego.RuntimeTypeAndRep {
			session.PutInt(x, 2, true, false)
		}
		session.PutBytes(0, 0)
	}
}
func (nego *DataTypeNego) write() error {
	session := nego.conn.session
	session.ResetBuffer()
	nego.writeMessage()
	return session.Write()
}

func TZBytes() []byte {
	_, offset := time.Now().Zone()
	hours := offset / 3600
	minutes := (offset / 60) % 60
	seconds := offset % 60
	return []byte{128, 0, 0, 0, byte(hours + 60), byte(minutes + 60), byte(seconds + 60), 128, 0, 0, 0}
}

// Wire datatype IDs from go-ora v3.0.1; these advertise representations only.
var typeRepresentations = [...]uint16{
	1, 1, 1, 0, 2, 2, 10, 0, 8, 8, 1, 0, 12, 12, 10, 0,
	23, 23, 1, 0, 24, 24, 1, 0, 25, 25, 1, 0, 26, 26, 1, 0,
	27, 27, 1, 0, 28, 28, 1, 0, 29, 29, 1, 0, 30, 30, 1, 0,
	31, 31, 1, 0, 32, 32, 1, 0, 33, 33, 1, 0, 10, 10, 1, 0,
	11, 11, 1, 0, 40, 40, 1, 0, 41, 41, 1, 0, 117, 117, 1, 0,
	120, 120, 1, 0, 290, 290, 1, 0, 291, 291, 1, 0, 292, 292, 1, 0,
	293, 293, 1, 0, 294, 294, 1, 0, 298, 298, 1, 0, 299, 299, 1, 0,
	300, 300, 1, 0, 301, 301, 1, 0, 302, 302, 1, 0, 303, 303, 1, 0,
	304, 304, 1, 0, 305, 305, 1, 0, 306, 306, 1, 0, 307, 307, 1, 0,
	308, 308, 1, 0, 309, 309, 1, 0, 310, 310, 1, 0, 311, 311, 1, 0,
	312, 312, 1, 0, 313, 313, 1, 0, 315, 315, 1, 0, 316, 316, 1, 0,
	317, 317, 1, 0, 318, 318, 1, 0, 319, 319, 1, 0, 320, 320, 1, 0,
	321, 321, 1, 0, 322, 322, 1, 0, 323, 323, 1, 0, 327, 327, 1, 0,
	328, 328, 1, 0, 329, 329, 1, 0, 331, 331, 1, 0, 333, 333, 1, 0,
	334, 334, 1, 0, 335, 335, 1, 0, 336, 336, 1, 0, 337, 337, 1, 0,
	338, 338, 1, 0, 339, 339, 1, 0, 340, 340, 1, 0, 341, 341, 1, 0,
	342, 342, 1, 0, 343, 343, 1, 0, 344, 344, 1, 0, 345, 345, 1, 0,
	346, 346, 1, 0, 348, 348, 1, 0, 349, 349, 1, 0, 354, 354, 1, 0,
	355, 355, 1, 0, 359, 359, 1, 0, 363, 363, 1, 0, 380, 380, 1, 0,
	381, 381, 1, 0, 382, 382, 1, 0, 383, 383, 1, 0, 384, 384, 1, 0,
	385, 385, 1, 0, 386, 386, 1, 0, 387, 387, 1, 0, 388, 388, 1, 0,
	389, 389, 1, 0, 390, 390, 1, 0, 391, 391, 1, 0, 393, 393, 1, 0,
	394, 394, 1, 0, 395, 395, 1, 0, 396, 396, 1, 0, 397, 397, 1, 0,
	398, 398, 1, 0, 399, 399, 1, 0, 400, 400, 1, 0, 401, 401, 1, 0,
	404, 404, 1, 0, 405, 405, 1, 0, 406, 406, 1, 0, 407, 407, 1, 0,
	413, 413, 1, 0, 414, 414, 1, 0, 415, 415, 1, 0, 416, 416, 1, 0,
	417, 417, 1, 0, 418, 418, 1, 0, 419, 419, 1, 0, 420, 420, 1, 0,
	421, 421, 1, 0, 422, 422, 1, 0, 423, 423, 1, 0, 424, 424, 1, 0,
	425, 425, 1, 0, 426, 426, 1, 0, 427, 427, 1, 0, 429, 429, 1, 0,
	430, 430, 1, 0, 431, 431, 1, 0, 432, 432, 1, 0, 433, 433, 1, 0,
	449, 449, 1, 0, 450, 450, 1, 0, 454, 454, 1, 0, 455, 455, 1, 0,
	456, 456, 1, 0, 457, 457, 1, 0, 458, 458, 1, 0, 459, 459, 1, 0,
	460, 460, 1, 0, 461, 461, 1, 0, 462, 462, 1, 0, 463, 463, 1, 0,
	466, 466, 1, 0, 467, 467, 1, 0, 468, 468, 1, 0, 469, 469, 1, 0,
	470, 470, 1, 0, 471, 471, 1, 0, 472, 472, 1, 0, 473, 473, 1, 0,
	474, 474, 1, 0, 475, 475, 1, 0, 476, 476, 1, 0, 477, 477, 1, 0,
	478, 478, 1, 0, 479, 479, 1, 0, 480, 480, 1, 0, 481, 481, 1, 0,
	482, 482, 1, 0, 483, 483, 1, 0, 484, 484, 1, 0, 485, 485, 1, 0,
	486, 486, 1, 0, 490, 490, 1, 0, 491, 491, 1, 0, 492, 492, 1, 0,
	493, 493, 1, 0, 494, 494, 1, 0, 495, 495, 1, 0, 496, 496, 1, 0,
	498, 498, 1, 0, 499, 499, 1, 0, 500, 500, 1, 0, 501, 501, 1, 0,
	502, 502, 1, 0, 509, 509, 1, 0, 510, 510, 1, 0, 513, 513, 1, 0,
	514, 514, 1, 0, 516, 516, 1, 0, 517, 517, 1, 0, 518, 518, 1, 0,
	519, 519, 1, 0, 520, 520, 1, 0, 521, 521, 1, 0, 522, 522, 1, 0,
	523, 523, 1, 0, 524, 524, 1, 0, 525, 525, 1, 0, 526, 526, 1, 0,
	527, 527, 1, 0, 528, 528, 1, 0, 529, 529, 1, 0, 530, 530, 1, 0,
	531, 531, 1, 0, 532, 532, 1, 0, 533, 533, 1, 0, 534, 534, 1, 0,
	535, 535, 1, 0, 536, 536, 1, 0, 537, 537, 1, 0, 538, 538, 1, 0,
	539, 539, 1, 0, 540, 540, 1, 0, 541, 541, 1, 0, 542, 542, 1, 0,
	543, 543, 1, 0, 560, 560, 1, 0, 562, 562, 1, 0, 565, 565, 1, 0,
	572, 572, 1, 0, 573, 573, 1, 0, 574, 574, 1, 0, 575, 575, 1, 0,
	576, 576, 1, 0, 578, 578, 1, 0, 563, 563, 1, 0, 564, 564, 1, 0,
	579, 579, 1, 0, 580, 580, 1, 0, 581, 581, 1, 0, 582, 582, 1, 0,
	583, 583, 1, 0, 584, 584, 1, 0, 585, 585, 1, 0, 3, 2, 10, 0,
	4, 2, 10, 0, 5, 1, 1, 0, 6, 2, 10, 0, 7, 2, 10, 0,
	9, 1, 1, 0, 13, 0, 14, 0, 15, 23, 1, 0, 16, 0, 17, 0,
	18, 0, 19, 0, 20, 0, 21, 0, 22, 0, 39, 646, 1, 0, 58, 0,
	68, 2, 10, 0, 69, 0, 70, 0, 74, 0, 76, 0, 91, 2, 10, 0,
	94, 1, 1, 0, 95, 23, 1, 0, 96, 96, 1, 0, 97, 96, 1, 0,
	100, 100, 1, 0, 101, 101, 1, 0, 102, 102, 1, 0, 104, 11, 1, 0,
	105, 0, 106, 106, 1, 0, 108, 109, 1, 0, 109, 109, 1, 0, 110, 111,
	1, 0, 111, 111, 1, 0, 112, 112, 1, 0, 113, 113, 1, 0, 114, 114,
	1, 0, 115, 115, 1, 0, 116, 102, 1, 0, 118, 0, 119, 119, 1, 0,
	127, 127, 1, 0, 121, 0, 122, 0, 123, 0, 136, 0, 146, 146, 1, 0,
	147, 0, 152, 2, 10, 0, 153, 2, 10, 0, 154, 2, 10, 0, 155, 1,
	1, 0, 156, 12, 10, 0, 172, 2, 10, 0, 178, 178, 1, 0, 179, 179,
	1, 0, 180, 180, 1, 0, 181, 181, 1, 0, 182, 182, 1, 0, 183, 183,
	1, 0, 184, 12, 10, 0, 185, 185, 1, 0, 186, 186, 1, 0, 187, 187,
	1, 0, 188, 188, 1, 0, 189, 189, 1, 0, 190, 190, 1, 0, 191, 0,
	192, 0, 195, 112, 1, 0, 196, 113, 1, 0, 197, 114, 1, 0, 208, 208,
	1, 0, 209, 0, 198, 127, 1, 0, 231, 231, 1, 0, 232, 231, 1, 0,
	233, 233, 1, 0, 252, 252, 1, 0, 241, 109, 1, 0, 515, 0, 590, 590,
	1, 0, 591, 591, 1, 0, 592, 592, 1, 0, 613, 613, 1, 0, 614, 614,
	1, 0, 615, 615, 1, 0, 616, 616, 1, 0, 647, 647, 1, 0, 611, 611,
	1, 0, 612, 612, 1, 0, 617, 617, 1, 0, 639, 639, 1, 0, 593, 593,
	1, 0, 594, 594, 1, 0, 595, 595, 1, 0, 596, 596, 1, 0, 597, 597,
	1, 0, 598, 598, 1, 0, 599, 599, 1, 0, 600, 600, 1, 0, 601, 601,
	1, 0, 602, 602, 1, 0, 603, 603, 1, 0, 604, 604, 1, 0, 605, 605,
	1, 0, 622, 622, 1, 0, 623, 623, 1, 0, 624, 624, 1, 0, 625, 625,
	1, 0, 626, 626, 1, 0, 627, 627, 1, 0, 628, 628, 1, 0, 629, 629,
	1, 0, 630, 630, 1, 0, 631, 631, 1, 0, 632, 632, 1, 0, 637, 637,
	1, 0, 638, 638, 1, 0, 636, 636, 1, 0, 663, 663, 1, 0, 640, 640,
	1, 0, 899, 899, 1, 0, 900, 900, 1, 0, 901, 901, 1, 0, 646, 646,
	1, 0, 662, 662, 1, 0,
}
