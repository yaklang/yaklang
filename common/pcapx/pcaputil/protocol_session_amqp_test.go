package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func amqpProto() []byte {
	return []byte{'A', 'M', 'Q', 'P', 0, 0, 9, 1}
}

func amqpFrame(typ byte, ch uint16, payload []byte) []byte {
	w := make([]byte, 8+len(payload))
	w[0] = typ
	binary.BigEndian.PutUint16(w[1:3], ch)
	binary.BigEndian.PutUint32(w[3:7], uint32(len(payload)))
	copy(w[7:], payload)
	w[7+len(payload)] = 0xce
	return w
}

func amqpMethod(ch uint16, class, method uint16, args []byte) []byte {
	p := make([]byte, 4+len(args))
	binary.BigEndian.PutUint16(p[0:2], class)
	binary.BigEndian.PutUint16(p[2:4], method)
	copy(p[4:], args)
	return amqpFrame(1, ch, p)
}

func amqpShortstr(s string) []byte {
	return append([]byte{byte(len(s))}, s...)
}

func amqpLongstr(s string) []byte {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(s)))
	return append(n[:], s...)
}

func amqpStart() []byte {
	b, err := hex.DecodeString("01000000000031000a000a0009000000150770726f6475637453000000085261626269744d5100000005504c41494e00000005656e5f5553ce")
	if err != nil {
		panic(err)
	}
	return b
}

func amqpStartOK() []byte {
	args := amqpLongstr("")
	args = append(args, amqpShortstr("PLAIN")...)
	args = append(args, amqpLongstr("")...)
	args = append(args, amqpShortstr("en_US")...)
	return amqpMethod(0, 10, 11, args)
}

func amqpChannelOpen(ch uint16) []byte {
	return amqpMethod(ch, 20, 10, amqpShortstr(""))
}

func amqpChannelOpenOK(ch uint16) []byte {
	return amqpMethod(ch, 20, 11, amqpLongstr(""))
}

func amqpPublish(ch uint16, exchange, key string) []byte {
	args := []byte{0, 0}
	args = append(args, amqpShortstr(exchange)...)
	args = append(args, amqpShortstr(key)...)
	args = append(args, 0)
	return amqpMethod(ch, 60, 40, args)
}

func amqpDeliver(ch uint16, tag uint64, key string) []byte {
	args := amqpShortstr("ct")
	var t [8]byte
	binary.BigEndian.PutUint64(t[:], tag)
	args = append(args, t[:]...)
	args = append(args, 0)
	args = append(args, amqpShortstr("")...)
	args = append(args, amqpShortstr(key)...)
	return amqpMethod(ch, 60, 60, args)
}

func amqpHeader(ch uint16, body uint64) []byte {
	p := make([]byte, 14)
	binary.BigEndian.PutUint16(p[0:2], 60)
	binary.BigEndian.PutUint64(p[4:12], body)
	return amqpFrame(2, ch, p)
}

func amqpBody(ch uint16, data []byte) []byte {
	return amqpFrame(3, ch, data)
}

func amqpAck(ch uint16, tag uint64) []byte {
	var t [8]byte
	binary.BigEndian.PutUint64(t[:], tag)
	return amqpMethod(ch, 60, 80, append(t[:], 0))
}

func amqpNack(ch uint16, tag uint64) []byte {
	var t [8]byte
	binary.BigEndian.PutUint64(t[:], tag)
	return amqpMethod(ch, 60, 120, append(t[:], 0))
}

func amqpHeartbeat() []byte {
	return amqpFrame(8, 0, nil)
}

func amqpClose() []byte {
	args := []byte{0, 200}
	args = append(args, amqpShortstr("OK")...)
	args = append(args, 0, 0, 0, 0)
	return amqpMethod(0, 10, 50, args)
}

func amqpCloseOK() []byte {
	return amqpMethod(0, 10, 51, nil)
}

func TestProtocolSessionAMQPPublishDeliverAck(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	p := s.Probe(amqpProto())
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "amqp", p.Protocol)
	require.Equal(t, "0-9-1", p.Version)

	r := s.Feed(0, ts, amqpProto())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "protocol-header", r.Events[0].Session["Packet Name"])

	r = s.Feed(1, ts, amqpStart())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "connection.start", r.Events[0].Session["Packet Name"])
	require.Equal(t, byte(0), r.Events[0].Session["Version Major"])
	require.Equal(t, byte(9), r.Events[0].Session["Version Minor"])
	require.Equal(t, true, r.Events[0].Session["Outstanding"])

	r = s.Feed(0, ts, amqpStartOK())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "connection.start-ok", r.Events[0].Session["Packet Name"])
	require.Equal(t, true, r.Events[0].Session["Matched Request"])

	r = s.Feed(0, ts, amqpChannelOpen(1))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "channel.open", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint16(1), r.Events[0].Session["Channel"])

	r = s.Feed(1, ts, amqpChannelOpenOK(1))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Matched Request"])

	r = s.Feed(0, ts, amqpPublish(1, "", "rk"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "basic.publish", r.Events[0].Session["Packet Name"])
	require.Equal(t, "rk", r.Events[0].Session["Routing Key"])

	r = s.Feed(0, ts, amqpHeader(1, 5))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "content-header", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint64(5), r.Events[0].Session["Body Size"])
	require.Equal(t, "basic.publish", r.Events[0].Session["Content Method"])

	r = s.Feed(0, ts, amqpBody(1, []byte("he")))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "content-body", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint64(3), r.Events[0].Session["Remaining Body"])

	r = s.Feed(0, ts, amqpBody(1, []byte("llo")))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Body Complete"])

	r = s.Feed(1, ts, amqpDeliver(1, 1, "rk"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "basic.deliver", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint64(1), r.Events[0].Session["Delivery Tag"])
	require.Nil(t, s.Feed(1, ts, amqpHeader(1, 1)).Err)
	require.Nil(t, s.Feed(1, ts, amqpBody(1, []byte("x"))).Err)

	r = s.Feed(0, ts, amqpAck(1, 1))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "basic.ack", r.Events[0].Session["Packet Name"])
	require.Equal(t, "basic.deliver", r.Events[0].Session["Matched Request"])

	r = s.Feed(1, ts, amqpDeliver(1, 2, "rk"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Nil(t, s.Feed(1, ts, amqpHeader(1, 0)).Err)
	r = s.Feed(0, ts, amqpNack(1, 2))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "basic.nack", r.Events[0].Session["Packet Name"])
	require.Equal(t, "matched", r.Events[0].Session["Association Status"])

	r = s.Feed(0, ts, amqpHeartbeat())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "heartbeat", r.Events[0].Session["Packet Name"])

	r = s.Feed(0, ts, amqpClose())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "connection.close", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint16(200), r.Events[0].Session["Reply Code"])
	r = s.Feed(1, ts, amqpCloseOK())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Matched Request"])

	cut := s.Feed(0, ts, amqpHeartbeat()[:3])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)
}

func TestProtocolSessionAMQPFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Equal(t, ProbeReject, s.Probe([]byte{'A', 'M', 'Q', 'P', 0, 1, 0, 0}).Verdict)
	require.Equal(t, ProbeNeedMore, s.Probe([]byte{'A', 'M', 'Q'}).Verdict)
	require.NotEqual(t, "amqp", s.Probe(amqpStart()).Protocol)

	r := s.Feed(0, ts, []byte{'A', 'M', 'Q', 'P', 0, 1, 0, 0})
	require.True(t, r.Err != nil || r.State == "undetected" || len(r.Events) == 0 || r.Events[0].Status != "decoded")

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, amqpProto()).Err)
	r = s2.Feed(0, ts, amqpBody(1, []byte("x")))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrContextRequired, r.Err.Kind)

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s3.Feed(0, ts, amqpProto()).Err)
	bad := amqpFrame(9, 0, nil)
	r = s3.Feed(0, ts, bad)
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)

	s4, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r = s4.Feed(0, ts, amqpProto()[:4])
	require.True(t, r.NeedMore || r.Err != nil && r.Err.Kind == ErrNeedMore)
}

func TestProtocolSessionAMQPFragmentation(t *testing.T) {
	steps := []sessionStep{
		{0, amqpProto()},
		{1, amqpStart()},
		{0, amqpStartOK()},
		{0, amqpChannelOpen(1)},
		{1, amqpChannelOpenOK(1)},
		{0, amqpPublish(1, "", "rk")},
		{0, amqpHeader(1, 5)},
		{0, amqpBody(1, []byte("he"))},
		{0, amqpBody(1, []byte("llo"))},
		{1, amqpDeliver(1, 1, "rk")},
		{1, amqpHeader(1, 1)},
		{1, amqpBody(1, []byte("x"))},
		{0, amqpAck(1, 1)},
	}
	assertFragmentation(t, steps, func(chunk int) []string {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		ts := time.Unix(1, 0)
		var names []string
		for _, st := range steps {
			for w := st.wire; len(w) > 0; {
				n := len(w)
				if chunk > 0 {
					n = min(n, chunk)
				}
				r := s.Feed(st.dir, ts, w[:n])
				for _, e := range r.Events {
					if e.Status == "decoded" || e.Status == "deferred" {
						names = append(names, fmt.Sprint(e.Session["Packet Name"]))
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}
