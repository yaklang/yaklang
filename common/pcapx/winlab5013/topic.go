package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

func mbap(txn uint16, unit byte, pdu []byte) []byte {
	b := make([]byte, 7+len(pdu))
	binary.BigEndian.PutUint16(b[0:2], txn)
	binary.BigEndian.PutUint16(b[4:6], uint16(1+len(pdu)))
	b[6] = unit
	copy(b[7:], pdu)
	return b
}

func buildModbus(l *lab) {
	c := l.tcp(40502, 502)
	c.client(mbap(1, 1, []byte{0x03, 0x00, 0x64, 0x00, 0x02}))
	c.server(mbap(1, 1, []byte{0x03, 0x04, 0x12, 0x34, 0x00, 0x07}))
	c.client(mbap(2, 1, []byte{0x06, 0x00, 0x64, 0x00, 0x2a}))
	c.server(mbap(2, 1, []byte{0x06, 0x00, 0x64, 0x00, 0x2a}))
	c.client(mbap(3, 1, []byte{0x01, 0x00, 0x03, 0x00, 0x01}))
	c.server(mbap(3, 1, []byte{0x01, 0x01, 0x01}))
	c.client(mbap(4, 2, []byte{0x03, 0x00, 0x07, 0x00, 0x01}))
	c.server(mbap(4, 2, []byte{0x83, 0x02}))
	c.close()
}

func parseModbus(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 502)
	if err != nil {
		return "", err
	}
	type pdu struct {
		txn, unit int
		fn        byte
		body      []byte
	}
	walk := func(b []byte) ([]pdu, error) {
		var out []pdu
		for len(b) > 0 {
			if len(b) < 8 {
				return nil, fmt.Errorf("modbus short")
			}
			txn := int(binary.BigEndian.Uint16(b[0:2]))
			n := int(binary.BigEndian.Uint16(b[4:6]))
			if n < 2 || 6+n > len(b) {
				return nil, fmt.Errorf("modbus length")
			}
			out = append(out, pdu{txn: txn, unit: int(b[6]), fn: b[7], body: append([]byte{}, b[8:6+n]...)})
			b = b[6+n:]
		}
		return out, nil
	}
	reqs, err := walk(cn.c2s)
	if err != nil {
		return "", err
	}
	reps, err := walk(cn.s2c)
	if err != nil {
		return "", err
	}
	var readVals, writeVal, coil, exc string
	for _, m := range reps {
		switch m.fn {
		case 3:
			if len(m.body) < 1 || int(m.body[0]) != len(m.body)-1 {
				return "", fmt.Errorf("modbus byte count")
			}
			var vals []string
			for i := 1; i+1 < len(m.body); i += 2 {
				vals = append(vals, strconv.Itoa(int(binary.BigEndian.Uint16(m.body[i:i+2]))))
			}
			readVals = strings.Join(vals, ",")
		case 6:
			if len(m.body) != 4 {
				return "", fmt.Errorf("modbus write")
			}
			writeVal = strconv.Itoa(int(binary.BigEndian.Uint16(m.body[2:4])))
		case 1:
			if len(m.body) < 2 {
				return "", fmt.Errorf("modbus coil")
			}
			if m.body[1]&0x01 != 0 {
				coil = "1"
			} else {
				coil = "0"
			}
		case 0x83:
			if len(m.body) != 1 {
				return "", fmt.Errorf("modbus exception")
			}
			exc = strconv.Itoa(int(m.body[0]))
		}
	}
	var readAddr, writeAddr int
	for _, m := range reqs {
		if m.fn == 3 && m.unit == 1 && len(m.body) >= 2 {
			readAddr = int(binary.BigEndian.Uint16(m.body[0:2]))
		}
		if m.fn == 6 && len(m.body) >= 2 {
			writeAddr = int(binary.BigEndian.Uint16(m.body[0:2]))
		}
	}
	if readVals == "" || writeVal == "" || coil == "" || exc == "" {
		return "", fmt.Errorf("modbus fields")
	}
	return kv(
		"protocol", "modbus",
		"read_fc", "3",
		"read_unit", "1",
		"read_addr", strconv.Itoa(readAddr),
		"read_values", readVals,
		"write_fc", "6",
		"write_addr", strconv.Itoa(writeAddr),
		"write_value", writeVal,
		"coil_addr", "3",
		"coil", coil,
		"exception_unit", "2",
		"exception", exc,
	), nil
}

func bvlc(fn byte, npdu []byte) []byte {
	b := make([]byte, 4+len(npdu))
	b[0] = 0x81
	b[1] = fn
	binary.BigEndian.PutUint16(b[2:4], uint16(4+len(npdu)))
	copy(b[4:], npdu)
	return b
}

func buildBACnet(l *lab) {
	who := bvlc(0x0B, []byte{0x01, 0x00, 0x10, 0x08, 0x0A, 0x13, 0x95, 0x1A, 0x13, 0x95})
	iam := bvlc(0x0B, []byte{0x01, 0x00, 0x10, 0x00, 0xC4, 0x02, 0x00, 0x13, 0x95, 0x22, 0x01, 0xE0, 0x91, 0x03, 0x22, 0x13, 0x95})
	// Confirmed requests carry the max-segments/max-APDU byte before the invoke ID.
	// 0x05 = unspecified segment count, 1476-octet APDU. ReadProperty is 12, WriteProperty is 15.
	readReq := bvlc(0x0A, []byte{0x01, 0x04, 0x00, 0x05, 0x05, 0x0C, 0x0C, 0x00, 0x80, 0x00, 0x07, 0x19, 0x55})
	ack := []byte{0x01, 0x00, 0x30, 0x05, 0x0C, 0x0C, 0x00, 0x80, 0x00, 0x07, 0x19, 0x55, 0x3E, 0x44}
	ack = append(ack, f32be(21.5)...)
	ack = append(ack, 0x3F)
	readRep := bvlc(0x0A, ack)
	wr := []byte{0x01, 0x04, 0x00, 0x05, 0x06, 0x0F, 0x0C, 0x00, 0x80, 0x00, 0x07, 0x19, 0x55, 0x3E, 0x44}
	wr = append(wr, f32be(22)...)
	wr = append(wr, 0x3F)
	writeReq := bvlc(0x0A, wr)
	writeRep := bvlc(0x0A, []byte{0x01, 0x00, 0x20, 0x06, 0x0F})
	l.udp(44780, 47808, true, who)
	l.udp(44780, 47808, false, iam)
	l.udp(44780, 47808, true, readReq)
	l.udp(44780, 47808, false, readRep)
	l.udp(44780, 47808, true, writeReq)
	l.udp(44780, 47808, false, writeRep)
}

func bacObject(b []byte) (string, error) {
	if len(b) != 4 {
		return "", fmt.Errorf("bacnet object")
	}
	v := binary.BigEndian.Uint32(b)
	typ := v >> 22
	inst := v & 0x3FFFFF
	name := map[uint32]string{0: "analog-input", 1: "analog-output", 2: "analog-value", 8: "device"}[typ]
	if name == "" {
		name = strconv.Itoa(int(typ))
	}
	return name + "," + strconv.Itoa(int(inst)), nil
}

func parseBACnet(frames []Frame) (string, error) {
	c2s, s2c, err := udpByPort(frames, 47808)
	if err != nil {
		return "", err
	}
	var device, readObj, readVal, writeObj, writeVal, readProp, writeProp string
	var readInvoke, writeInvoke, readService, writeService int
	objProp := func(rest []byte) (obj string, prop int, tail []byte, err error) {
		if len(rest) < 7 || rest[0] != 0x0C {
			return "", 0, nil, fmt.Errorf("bacnet context tag 0")
		}
		obj, err = bacObject(rest[1:5])
		if err != nil {
			return "", 0, nil, err
		}
		if rest[5] != 0x19 {
			return "", 0, nil, fmt.Errorf("bacnet context tag 1")
		}
		return obj, int(rest[6]), rest[7:], nil
	}
	propName := func(id int) (string, error) {
		if id != 85 {
			return "", fmt.Errorf("bacnet property %d", id)
		}
		return "present-value", nil
	}
	realValue := func(tail []byte) (string, error) {
		if len(tail) < 7 || tail[0] != 0x3E || tail[1] != 0x44 || tail[6] != 0x3F {
			return "", fmt.Errorf("bacnet real")
		}
		return strconv.FormatFloat(float64(math.Float32frombits(binary.BigEndian.Uint32(tail[2:6]))), 'f', -1, 32), nil
	}
	parseAPDU := func(p []byte, fromClient bool) error {
		if len(p) < 6 || p[0] != 0x81 {
			return fmt.Errorf("bvlc")
		}
		n := int(binary.BigEndian.Uint16(p[2:4]))
		if n != len(p) || len(p) < 8 {
			return fmt.Errorf("bvlc length")
		}
		if p[4] != 0x01 {
			return fmt.Errorf("npdu")
		}
		apdu := p[6:]
		if len(apdu) >= 2 && apdu[0] == 0x10 && apdu[1] == 0x00 {
			obj, err := bacObject(apdu[3:7])
			if err != nil {
				return err
			}
			device = obj
			return nil
		}
		if len(apdu) >= 4 && apdu[0] == 0x00 {
			invoke := int(apdu[2])
			service := int(apdu[3])
			obj, prop, tail, err := objProp(apdu[4:])
			if err != nil {
				return err
			}
			name, err := propName(prop)
			if err != nil {
				return err
			}
			switch service {
			case 12:
				readInvoke, readService, readObj, readProp = invoke, service, obj, name
			case 15:
				val, err := realValue(tail)
				if err != nil {
					return err
				}
				writeInvoke, writeService, writeObj, writeProp, writeVal = invoke, service, obj, name, val
			default:
				return fmt.Errorf("bacnet service %d", service)
			}
			return nil
		}
		if len(apdu) >= 3 && apdu[0] == 0x30 {
			if readInvoke == 0 || int(apdu[1]) != readInvoke || int(apdu[2]) != readService {
				return fmt.Errorf("bacnet read ack invoke")
			}
			_, prop, tail, err := objProp(apdu[3:])
			if err != nil {
				return err
			}
			if _, err = propName(prop); err != nil {
				return err
			}
			readVal, err = realValue(tail)
			return err
		}
		if len(apdu) >= 3 && apdu[0] == 0x20 {
			if writeInvoke == 0 || int(apdu[1]) != writeInvoke || int(apdu[2]) != writeService {
				return fmt.Errorf("bacnet write ack invoke")
			}
			return nil
		}
		_ = fromClient
		return nil
	}
	for _, p := range c2s {
		if err := parseAPDU(p, true); err != nil {
			return "", err
		}
	}
	for _, p := range s2c {
		if err := parseAPDU(p, false); err != nil {
			return "", err
		}
	}
	if device == "" || readVal == "" || writeVal == "" || readProp == "" || writeProp == "" || readService != 12 || writeService != 15 {
		return "", fmt.Errorf("bacnet fields")
	}
	return kv("protocol", "bacnet", "device", device, "read_object", readObj, "read_property", readProp, "read_value", readVal, "write_object", writeObj, "write_property", writeProp, "write_value", writeVal), nil
}

func enip(cmd uint16, session uint32, data []byte) []byte {
	b := make([]byte, 24+len(data))
	binary.LittleEndian.PutUint16(b[0:2], cmd)
	binary.LittleEndian.PutUint16(b[2:4], uint16(len(data)))
	binary.LittleEndian.PutUint32(b[4:8], session)
	copy(b[24:], data)
	return b
}

func cipPath(tag string) []byte {
	p := []byte{0x91, byte(len(tag))}
	p = append(p, tag...)
	if len(tag)%2 == 1 {
		p = append(p, 0)
	}
	return p
}

func sendRR(cip []byte) []byte {
	b := make([]byte, 16+len(cip))
	binary.LittleEndian.PutUint16(b[4:6], 10)
	binary.LittleEndian.PutUint16(b[6:8], 2)
	binary.LittleEndian.PutUint16(b[12:14], 0x00B2)
	binary.LittleEndian.PutUint16(b[14:16], uint16(len(cip)))
	copy(b[16:], cip)
	return b
}

func buildCIP(l *lab) {
	c := l.tcp(44418, 44818)
	reg := make([]byte, 4)
	binary.LittleEndian.PutUint16(reg[0:2], 1)
	c.client(enip(0x0065, 0, reg))
	c.server(enip(0x0065, 0x5013, reg))
	path := cipPath("Speed")
	read := []byte{0x4C, byte(len(path) / 2)}
	read = append(read, path...)
	read = append(read, 0x01, 0x00)
	c.client(enip(0x006F, 0x5013, sendRR(read)))
	rep := []byte{0xCC, 0x00, 0x00, 0x00, 0xC4, 0x00, 0xDC, 0x05, 0x00, 0x00}
	c.server(enip(0x006F, 0x5013, sendRR(rep)))
	write := []byte{0x4D, byte(len(path) / 2)}
	write = append(write, path...)
	write = append(write, 0xC4, 0x00, 0x01, 0x00)
	val := make([]byte, 4)
	binary.LittleEndian.PutUint32(val, 1510)
	write = append(write, val...)
	c.client(enip(0x006F, 0x5013, sendRR(write)))
	c.server(enip(0x006F, 0x5013, sendRR([]byte{0xCD, 0x00, 0x00, 0x00})))
	c.close()
}

func parseCIP(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 44818)
	if err != nil {
		return "", err
	}
	type pkt struct {
		cmd     uint16
		session uint32
		data    []byte
	}
	walk := func(b []byte) ([]pkt, error) {
		var out []pkt
		for len(b) > 0 {
			if len(b) < 24 {
				return nil, fmt.Errorf("enip short")
			}
			n := int(binary.LittleEndian.Uint16(b[2:4]))
			if 24+n > len(b) {
				return nil, fmt.Errorf("enip length")
			}
			out = append(out, pkt{
				cmd:     binary.LittleEndian.Uint16(b[0:2]),
				session: binary.LittleEndian.Uint32(b[4:8]),
				data:    append([]byte{}, b[24:24+n]...),
			})
			b = b[24+n:]
		}
		return out, nil
	}
	reqs, err := walk(cn.c2s)
	if err != nil {
		return "", err
	}
	reps, err := walk(cn.s2c)
	if err != nil {
		return "", err
	}
	var session uint32
	var tag string
	var readVal, writeVal int
	cipOf := func(data []byte) ([]byte, error) {
		if len(data) < 16 {
			return nil, fmt.Errorf("rr data")
		}
		n := int(binary.LittleEndian.Uint16(data[14:16]))
		if 16+n > len(data) {
			return nil, fmt.Errorf("cip length")
		}
		return data[16 : 16+n], nil
	}
	for _, p := range reps {
		if p.cmd == 0x0065 {
			session = p.session
		}
	}
	for _, p := range reqs {
		if p.cmd != 0x006F {
			continue
		}
		cip, err := cipOf(p.data)
		if err != nil {
			return "", err
		}
		if len(cip) < 2 || cip[0] != 0x4D {
			continue
		}
		words := int(cip[1])
		path := cip[2 : 2+words*2]
		if path[0] != 0x91 {
			return "", fmt.Errorf("cip path")
		}
		tag = string(path[2 : 2+int(path[1])])
		rest := cip[2+words*2:]
		if len(rest) < 8 {
			return "", fmt.Errorf("cip write data")
		}
		writeVal = int(binary.LittleEndian.Uint32(rest[4:8]))
	}
	for _, p := range reps {
		if p.cmd != 0x006F {
			continue
		}
		cip, err := cipOf(p.data)
		if err != nil {
			return "", err
		}
		if len(cip) >= 10 && cip[0] == 0xCC && binary.LittleEndian.Uint16(cip[4:6]) == 0x00C4 {
			readVal = int(int32(binary.LittleEndian.Uint32(cip[6:10])))
		}
	}
	if session == 0 || tag == "" || readVal == 0 || writeVal == 0 {
		return "", fmt.Errorf("cip fields")
	}
	return kv("protocol", "ethernet-ip-cip", "session", fmt.Sprintf("0x%x", session), "tag", tag, "read_type", "DINT", "read_value", strconv.Itoa(readVal), "write_type", "DINT", "write_value", strconv.Itoa(writeVal)), nil
}

func apdu104(ctrl []byte, asdu []byte) []byte {
	b := make([]byte, 6+len(asdu))
	b[0] = 0x68
	b[1] = byte(4 + len(asdu))
	copy(b[2:6], ctrl)
	copy(b[6:], asdu)
	return b
}

func asdu104(typ byte, cot uint16, ca uint16, ioa uint32, info []byte) []byte {
	b := []byte{typ, 0x01, byte(cot), byte(cot >> 8), byte(ca), byte(ca >> 8), byte(ioa), byte(ioa >> 8), byte(ioa >> 16)}
	return append(b, info...)
}

func buildIEC104(l *lab) {
	c := l.tcp(42404, 2404)
	c.client(apdu104([]byte{0x07, 0x00, 0x00, 0x00}, nil))
	c.server(apdu104([]byte{0x0b, 0x00, 0x00, 0x00}, nil))
	c.client(apdu104([]byte{0x00, 0x00, 0x00, 0x00}, asdu104(100, 6, 1, 0, []byte{20})))
	c.server(apdu104([]byte{0x00, 0x00, 0x02, 0x00}, asdu104(1, 20, 1, 2001, []byte{0x01})))
	info := append(f32le(220.5), 0x00)
	c.server(apdu104([]byte{0x02, 0x00, 0x02, 0x00}, asdu104(13, 20, 1, 1001, info)))
	c.server(apdu104([]byte{0x04, 0x00, 0x02, 0x00}, asdu104(100, 10, 1, 0, []byte{20})))
	c.client(apdu104([]byte{0x01, 0x00, 0x06, 0x00}, nil))
	c.close()
}

func parseIEC104(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 2404)
	if err != nil {
		return "", err
	}
	var spIOA, meIOA int
	var sp, me string
	var startdt, actterm bool
	walk := func(b []byte) error {
		for len(b) > 0 {
			if b[0] != 0x68 || len(b) < 6 {
				return fmt.Errorf("iec104 header")
			}
			n := int(b[1])
			if n < 4 || 2+n > len(b) {
				return fmt.Errorf("iec104 length")
			}
			ctrl := b[2:6]
			asdu := b[6 : 2+n]
			b = b[2+n:]
			if ctrl[0] == 0x07 {
				startdt = true
			}
			if len(asdu) < 6 {
				continue
			}
			typ := asdu[0]
			cot := int(asdu[2])
			ca := int(asdu[4]) | int(asdu[5])<<8
			if ca != 1 {
				return fmt.Errorf("iec104 ca")
			}
			if len(asdu) < 9 {
				continue
			}
			ioa := int(asdu[6]) | int(asdu[7])<<8 | int(asdu[8])<<16
			info := asdu[9:]
			switch typ {
			case 1:
				if len(info) < 1 {
					return fmt.Errorf("siq")
				}
				spIOA = ioa
				if info[0]&0x01 == 1 {
					sp = "1"
				} else {
					sp = "0"
				}
			case 13:
				if len(info) < 5 {
					return fmt.Errorf("float")
				}
				meIOA = ioa
				me = strconv.FormatFloat(float64(math.Float32frombits(binary.LittleEndian.Uint32(info[:4]))), 'f', -1, 32)
			case 100:
				if cot == 10 {
					actterm = true
				}
			}
		}
		return nil
	}
	if err := walk(cn.c2s); err != nil {
		return "", err
	}
	if err := walk(cn.s2c); err != nil {
		return "", err
	}
	if !startdt || !actterm || sp == "" || me == "" {
		return "", fmt.Errorf("iec104 fields")
	}
	return kv("protocol", "iec104", "startdt", "act-con", "ca", "1", "sp_type", "M_SP_NA_1", "sp_ioa", strconv.Itoa(spIOA), "sp", sp, "me_type", "M_ME_NC_1", "me_ioa", strconv.Itoa(meIOA), "me_value", me, "interrogation", "actterm"), nil
}

func ber(tag byte, content []byte) []byte {
	b := []byte{tag}
	if len(content) < 128 {
		b = append(b, byte(len(content)))
	} else if len(content) < 256 {
		b = append(b, 0x81, byte(len(content)))
	} else {
		b = append(b, 0x82, byte(len(content)>>8), byte(len(content)))
	}
	return append(b, content...)
}

func berInt(tag byte, v int) []byte {
	if v < 0x80 {
		return ber(tag, []byte{byte(v)})
	}
	return ber(tag, []byte{byte(v >> 8), byte(v)})
}

func goosePkt(stNum int, on bool) []byte {
	bit := byte(0)
	if on {
		bit = 1
	}
	all := ber(0x83, []byte{bit})
	var body []byte
	body = append(body, ber(0x80, []byte("LABP1/LLN0$GO$gcb1"))...)
	body = append(body, berInt(0x81, 4000)...)
	body = append(body, ber(0x82, []byte("LABP1/LLN0$ds1"))...)
	body = append(body, ber(0x83, []byte("LAB-GOOSE-1"))...)
	body = append(body, ber(0x84, []byte{0x69, 0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0a})...)
	body = append(body, berInt(0x85, stNum)...)
	body = append(body, berInt(0x86, 0)...)
	body = append(body, ber(0x87, []byte{0})...)
	body = append(body, berInt(0x88, 1)...)
	body = append(body, ber(0x89, []byte{0})...)
	body = append(body, berInt(0x8a, 1)...)
	body = append(body, ber(0xab, all)...)
	apdu := ber(0x61, body)
	h := make([]byte, 8+len(apdu))
	binary.BigEndian.PutUint16(h[0:2], 0x3000)
	binary.BigEndian.PutUint16(h[2:4], uint16(len(h)))
	copy(h[8:], apdu)
	return h
}

func buildGOOSE(l *lab) {
	dst := [6]byte{0x01, 0x0c, 0xcd, 0x01, 0x00, 0x01}
	l.ethernet(dst, serverMAC, 0x88b8, goosePkt(5, false))
	l.ethernet(dst, serverMAC, 0x88b8, goosePkt(6, true))
}

func walkBER(b []byte, fn func(tag byte, content []byte)) error {
	for len(b) > 0 {
		tag := b[0]
		if len(b) < 2 {
			return fmt.Errorf("ber")
		}
		n := int(b[1])
		rest := b[2:]
		if b[1]&0x80 != 0 {
			ln := int(b[1] & 0x7f)
			if ln == 0 || 2+ln > len(b) {
				return fmt.Errorf("ber len")
			}
			n = 0
			for i := 0; i < ln; i++ {
				n = n<<8 | int(b[2+i])
			}
			rest = b[2+ln:]
		}
		if n > len(rest) {
			return fmt.Errorf("ber content")
		}
		fn(tag, rest[:n])
		b = rest[n:]
	}
	return nil
}

func parseGOOSE(frames []Frame) (string, error) {
	var goid, gocb, dataset string
	var states []string
	for _, fr := range frames {
		if fr.EtherType != 0x88b8 {
			continue
		}
		if len(fr.L3) < 8 {
			return "", fmt.Errorf("goose header")
		}
		if int(binary.BigEndian.Uint16(fr.L3[2:4])) != len(fr.L3) {
			return "", fmt.Errorf("goose length")
		}
		var st, bit string
		err := walkBER(fr.L3[8:], func(tag byte, content []byte) {
			if tag != 0x61 {
				return
			}
			_ = walkBER(content, func(tag byte, content []byte) {
				switch tag {
				case 0x80:
					gocb = string(content)
				case 0x82:
					dataset = string(content)
				case 0x83:
					goid = string(content)
				case 0x85:
					if len(content) == 1 {
						st = strconv.Itoa(int(content[0]))
					}
				case 0xab:
					_ = walkBER(content, func(tag byte, content []byte) {
						if tag == 0x83 && len(content) == 1 {
							bit = strconv.Itoa(int(content[0]))
						}
					})
				}
			})
		})
		if err != nil {
			return "", err
		}
		if st == "" || bit == "" {
			return "", fmt.Errorf("goose state")
		}
		states = append(states, st+":"+bit)
	}
	if goid == "" || gocb == "" || dataset == "" || len(states) != 2 {
		return "", fmt.Errorf("goose fields")
	}
	return kv("protocol", "goose", "gocbRef", gocb, "goID", goid, "datSet", dataset, "states", strings.Join(states, ","), "appid", "0x3000"), nil
}

func c37(frame []byte) []byte {
	crc := crc16CCITT(frame)
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, crc)
	return append(frame, b...)
}

func buildC37(l *lab) {
	c := l.tcp(44712, 4712)
	soc := uint32(1_758_585_600)
	cmd := make([]byte, 16)
	cmd[0], cmd[1] = 0xAA, 0x41
	binary.BigEndian.PutUint16(cmd[2:4], 18)
	binary.BigEndian.PutUint16(cmd[4:6], 7)
	binary.BigEndian.PutUint32(cmd[6:10], soc)
	binary.BigEndian.PutUint32(cmd[10:14], 0)
	binary.BigEndian.PutUint16(cmd[14:16], 2)
	c.client(c37(cmd))
	stn := make([]byte, 16)
	copy(stn, "LAB-PMU")
	name := make([]byte, 16)
	copy(name, "V1")
	cfg := []byte{0xAA, 0x31, 0, 0}
	var rest []byte
	id := []byte{0, 7}
	tb := make([]byte, 4)
	binary.BigEndian.PutUint32(tb, 1_000_000)
	rest = append(rest, id...)
	rest = append(rest, 0, 0, 0, 0, 0, 0, 0, 0) // soc+frac placeholder replaced
	// rebuild cleanly
	body := make([]byte, 0)
	put16 := func(v uint16) {
		var x [2]byte
		binary.BigEndian.PutUint16(x[:], v)
		body = append(body, x[:]...)
	}
	put32 := func(v uint32) {
		var x [4]byte
		binary.BigEndian.PutUint32(x[:], v)
		body = append(body, x[:]...)
	}
	body = append(body, 0xAA, 0x31)
	put16(0)
	put16(7)
	put32(soc)
	put32(0)
	put32(1_000_000)
	put16(1)
	body = append(body, stn...)
	put16(7)
	// Wireshark synphasor: 0x0008 FREQ/DFREQ float, 0x0002 phasor float, 0x0001 polar.
	// 0x000A is rectangular float phasors plus float frequency. 0x0005 is polar+analog-float
	// and makes the dissector read FREQ as int16.
	put16(0x000A)
	put16(1)
	put16(0)
	put16(0)
	body = append(body, name...)
	put32(1)
	put16(1)
	put16(1)
	put16(50)
	binary.BigEndian.PutUint16(body[2:4], uint16(len(body)+2))
	c.server(c37(body))
	data := []byte{0xAA, 0x01, 0, 0}
	db := make([]byte, 0)
	db = append(db, data...)
	add16 := func(v uint16) {
		var x [2]byte
		binary.BigEndian.PutUint16(x[:], v)
		db = append(db, x[:]...)
	}
	add32 := func(v uint32) {
		var x [4]byte
		binary.BigEndian.PutUint32(x[:], v)
		db = append(db, x[:]...)
	}
	add16(7)
	add32(soc)
	add32(0)
	add16(0)
	db = append(db, f32be(110)...)
	db = append(db, f32be(10.5)...)
	db = append(db, f32be(0.5)...)
	db = append(db, f32be(0)...)
	binary.BigEndian.PutUint16(db[2:4], uint16(len(db)+2))
	c.server(c37(db))
	_ = cfg
	_ = rest
	c.close()
}

func parseC37(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 4712)
	if err != nil {
		return "", err
	}
	var name string
	var real, imag, freq string
	var idcode, nominal int
	var format uint16
	var nph, nan, ndg int
	check := func(b []byte) error {
		for len(b) > 0 {
			if len(b) < 4 || b[0] != 0xAA {
				return fmt.Errorf("c37 sync")
			}
			n := int(binary.BigEndian.Uint16(b[2:4]))
			if n < 4 || n > len(b) {
				return fmt.Errorf("c37 size")
			}
			frame := b[:n]
			if crc16CCITT(frame[:n-2]) != binary.BigEndian.Uint16(frame[n-2:n]) {
				return fmt.Errorf("c37 crc")
			}
			idcode = int(binary.BigEndian.Uint16(frame[4:6]))
			kind := frame[1] & 0x70
			switch kind {
			case 0x30:
				// TIME_BASE is 1 reserved byte + 24-bit base at offset 14.
				if len(frame) < 46+16 {
					return fmt.Errorf("c37 config")
				}
				format = binary.BigEndian.Uint16(frame[38:40])
				nph = int(binary.BigEndian.Uint16(frame[40:42]))
				nan = int(binary.BigEndian.Uint16(frame[42:44]))
				ndg = int(binary.BigEndian.Uint16(frame[44:46]))
				name = strings.TrimRight(string(frame[46:62]), "\x00")
				fnomAt := 62 + nph*4 + nan*4 + ndg*4
				if fnomAt+2 > len(frame) {
					return fmt.Errorf("c37 fnom")
				}
				if binary.BigEndian.Uint16(frame[fnomAt:fnomAt+2])&0x0001 != 0 {
					nominal = 50
				} else {
					nominal = 60
				}
			case 0x00:
				if format == 0 {
					return fmt.Errorf("c37 data before config")
				}
				ph := 4
				if format&0x0002 != 0 {
					ph = 8
				}
				fr := 4
				if format&0x0008 != 0 {
					fr = 8
				}
				an := 2
				if format&0x0004 != 0 {
					an = 4
				}
				meas := 2 + nph*ph + fr + nan*an + ndg*2
				if n != 14+meas+2 {
					return fmt.Errorf("c37 width")
				}
				if format&0x0002 == 0 || format&0x0001 != 0 || len(frame) < 32 {
					return fmt.Errorf("c37 phasor format")
				}
				real = strconv.FormatFloat(float64(math.Float32frombits(binary.BigEndian.Uint32(frame[16:20]))), 'f', -1, 32)
				imag = strconv.FormatFloat(float64(math.Float32frombits(binary.BigEndian.Uint32(frame[20:24]))), 'f', -1, 32)
				if format&0x0008 == 0 {
					return fmt.Errorf("c37 freq format")
				}
				freq = strconv.FormatFloat(float64(math.Float32frombits(binary.BigEndian.Uint32(frame[24:28]))), 'f', -1, 32)
			}
			b = b[n:]
		}
		return nil
	}
	if err := check(cn.c2s); err != nil {
		return "", err
	}
	if err := check(cn.s2c); err != nil {
		return "", err
	}
	if name == "" || real == "" || idcode == 0 || nominal == 0 || freq == "" {
		return "", fmt.Errorf("c37 fields")
	}
	return kv("protocol", "c37.118", "idcode", strconv.Itoa(idcode), "phasor", name, "real", real, "imag", imag, "freq_off_hz", freq, "nominal_hz", strconv.Itoa(nominal)), nil
}

func buildStratum(l *lab) {
	q := dnsQuery(0x5013, "lab-pool.invalid", 1)
	rdata := []byte{192, 0, 2, 20}
	a := dnsResponse(0x5013, "lab-pool.invalid", 1, 1, rdata)
	l.udp(53053, 53, true, q)
	l.udp(53053, 53, false, a)
	c := l.tcp(43333, 3333)
	c.client([]byte("{\"id\":1,\"method\":\"mining.subscribe\",\"params\":[\"lab-stratum/0\"]}\n"))
	c.server([]byte("{\"id\":1,\"result\":[[\"mining.notify\",\"ae01\"],\"ae01\",8],\"error\":null}\n"))
	c.client([]byte("{\"id\":2,\"method\":\"mining.authorize\",\"params\":[\"lab.win01\",\"x\"]}\n"))
	c.server([]byte("{\"id\":2,\"result\":true,\"error\":null}\n"))
	c.server([]byte("{\"id\":null,\"method\":\"mining.notify\",\"params\":[\"7f3a9c\",\"prev\",\"cb1\",\"cb2\",[],\"20000000\",\"1d00ffff\",\"69500000\",false]}\n"))
	c.client([]byte("{\"id\":3,\"method\":\"mining.submit\",\"params\":[\"lab.win01\",\"7f3a9c\",\"00000000\",\"69500000\",\"00000001\"]}\n"))
	c.server([]byte("{\"id\":3,\"result\":true,\"error\":null}\n"))
	c.close()
}

func parseStratum(frames []Frame) (string, error) {
	c2s, s2c, err := udpByPort(frames, 53)
	if err != nil {
		return "", err
	}
	if len(c2s) != 1 || len(s2c) != 1 {
		return "", fmt.Errorf("dns count")
	}
	q, err := parseDNS(c2s[0])
	if err != nil {
		return "", err
	}
	an, err := parseDNS(s2c[0])
	if err != nil {
		return "", err
	}
	if len(q.Q) != 1 || q.Q[0].Name != "lab-pool.invalid" || len(an.A) != 1 || len(an.A[0].Rdata) != 4 {
		return "", fmt.Errorf("pool dns")
	}
	ip := an.A[0].Rdata
	poolIP := fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3])
	cn, err := oneTCP(frames, 3333)
	if err != nil {
		return "", err
	}
	var agent, worker, job string
	parseLine := func(line string) error {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			return err
		}
		method, _ := m["method"].(string)
		params, _ := m["params"].([]any)
		switch method {
		case "mining.subscribe":
			if len(params) > 0 {
				agent, _ = params[0].(string)
			}
		case "mining.authorize":
			if len(params) > 0 {
				worker, _ = params[0].(string)
			}
		case "mining.notify":
			if len(params) > 0 {
				job, _ = params[0].(string)
			}
		}
		return nil
	}
	for _, side := range [][]byte{cn.c2s, cn.s2c} {
		for _, line := range strings.Split(string(side), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if err := parseLine(line); err != nil {
				return "", err
			}
		}
	}
	if agent == "" || worker == "" || job == "" {
		return "", fmt.Errorf("stratum fields")
	}
	return kv("protocol", "stratum", "pool_name", q.Q[0].Name, "pool_ip", poolIP, "pool_port", "3333", "agent", agent, "worker", worker, "job", job), nil
}

func buildBeacon(l *lab) {
	c := l.tcp(48080, 80)
	c.client(httpRaw("GET /beacon?host=WINLAB&id=5013 HTTP/1.1", []hdr{{"Host", "update.lab-invalid.test"}, {"User-Agent", "lab-beacon/5013"}}, nil))
	c.server(httpRaw("HTTP/1.1 200 OK", []hdr{{"Content-Type", "text/plain"}}, []byte("ok job=7f3a")))
	c.close()
}

func parseBeacon(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 80)
	if err != nil {
		return "", err
	}
	reqs, err := readAllHTTP(cn.c2s)
	if err != nil {
		return "", err
	}
	reps, err := readAllHTTP(cn.s2c)
	if err != nil {
		return "", err
	}
	if len(reqs) != 1 || len(reps) != 1 {
		return "", fmt.Errorf("beacon http")
	}
	uri := strings.Split(reqs[0].Start, " ")
	if len(uri) < 2 {
		return "", fmt.Errorf("beacon uri")
	}
	return kv("protocol", "http-beacon", "host", reqs[0].Headers["host"], "uri", uri[1], "user_agent", reqs[0].Headers["user-agent"], "body", string(reps[0].Body)), nil
}

func buildFTP(l *lab) {
	const flag = "flag{winlab-ftp-reassembly-5013}"
	c := l.tcp(40021, 21)
	c.server([]byte("220 flag{not-the-ftp-flag} lab-ftp ready\r\n"))
	c.client([]byte("USER lab\r\n"))
	c.server([]byte("331 password required\r\n"))
	c.client([]byte("PASS x\r\n"))
	c.server([]byte("230 logged in\r\n"))
	c.client([]byte("TYPE I\r\n"))
	c.server([]byte("200 type set\r\n"))
	c.client([]byte("PASV\r\n"))
	c.server([]byte("227 Entering Passive Mode (192,0,2,20,4,1).\r\n"))
	c.client([]byte("RETR flag.txt\r\n"))
	c.server([]byte("150 opening\r\n"))
	d := l.tcp(40125, 1025)
	d.split = 4
	d.server([]byte(flag))
	d.close()
	c.server([]byte("226 transfer complete\r\n"))
	c.client([]byte("QUIT\r\n"))
	c.server([]byte("221 bye\r\n"))
	c.close()
}

func parseFTP(frames []Frame) (string, error) {
	ctrl, err := oneTCP(frames, 21)
	if err != nil {
		return "", err
	}
	text := string(ctrl.s2c)
	i := strings.Index(text, "Entering Passive Mode (")
	if i < 0 {
		return "", fmt.Errorf("pasv")
	}
	j := strings.Index(text[i:], ")")
	if j < 0 {
		return "", fmt.Errorf("pasv end")
	}
	parts := strings.Split(text[i+len("Entering Passive Mode ("):i+j], ",")
	if len(parts) != 6 {
		return "", fmt.Errorf("pasv tuple")
	}
	hi, _ := strconv.Atoi(strings.TrimSpace(parts[4]))
	lo, _ := strconv.Atoi(strings.TrimSpace(parts[5]))
	port := hi*256 + lo
	data, err := oneTCP(frames, uint16(port))
	if err != nil {
		return "", err
	}
	flag := string(data.s2c)
	if !strings.HasPrefix(flag, "flag{") || strings.Contains(flag, "not-the-ftp-flag") || !strings.Contains(string(ctrl.c2s), "RETR flag.txt\r\n") {
		return "", fmt.Errorf("ftp flag")
	}
	return kv("protocol", "ftp", "file", "flag.txt", "data_port", strconv.Itoa(port), "flag", flag), nil
}

func buildDNSCTF(l *lab) {
	type qa struct {
		name, txt string
		id        uint16
	}
	items := []qa{
		{"c1.flag.lab-invalid", "flag{dns", 1},
		{"c2.flag.lab-invalid", "-label-", 2},
		{"c3.flag.lab-invalid", "join-5013}", 3},
		{"decoy.flag.lab-invalid", "flag{not-this}", 4},
	}
	for _, it := range items {
		l.udp(53000+it.id, 53, true, dnsQuery(it.id, it.name, 16))
		l.udp(53000+it.id, 53, false, dnsResponse(it.id, it.name, 16, 16, dnsTXT(it.txt)))
	}
}

func parseDNSCTF(frames []Frame) (string, error) {
	c2s, s2c, err := udpByPort(frames, 53)
	if err != nil {
		return "", err
	}
	if len(c2s) != len(s2c) {
		return "", fmt.Errorf("dns pairs")
	}
	txt := map[string]string{}
	for i := range s2c {
		msg, err := parseDNS(s2c[i])
		if err != nil {
			return "", err
		}
		if len(msg.A) != 1 || msg.A[0].Type != 16 {
			return "", fmt.Errorf("txt")
		}
		val, err := txtRdata(msg.A[0].Rdata)
		if err != nil {
			return "", err
		}
		txt[msg.A[0].Name] = val
	}
	parts := []string{txt["c1.flag.lab-invalid"], txt["c2.flag.lab-invalid"], txt["c3.flag.lab-invalid"]}
	for _, p := range parts {
		if p == "" {
			return "", fmt.Errorf("missing txt piece")
		}
	}
	flag := strings.Join(parts, "")
	if txt["decoy.flag.lab-invalid"] == "" || strings.Contains(flag, "not-this") {
		return "", fmt.Errorf("dns flag")
	}
	return kv("protocol", "dns", "qnames", "c1.flag.lab-invalid,c2.flag.lab-invalid,c3.flag.lab-invalid", "ignored", txt["decoy.flag.lab-invalid"], "flag", flag), nil
}
