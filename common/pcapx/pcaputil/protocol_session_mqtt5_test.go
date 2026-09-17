package pcaputil

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func mqttRemain(n int) []byte {
	var w []byte
	for {
		b := byte(n % 128)
		n /= 128
		if n != 0 {
			b |= 128
		}
		w = append(w, b)
		if n == 0 {
			return w
		}
	}
}

func mqttPkt(hdr byte, body []byte) []byte {
	return append(append([]byte{hdr}, mqttRemain(len(body))...), body...)
}

func mqttUTF(s string) []byte {
	w := make([]byte, 2+len(s))
	binary.BigEndian.PutUint16(w, uint16(len(s)))
	copy(w[2:], s)
	return w
}

func mqtt5Connect(id string, props []byte) []byte {
	if props == nil {
		props = []byte{0}
	}
	body := append(mqttUTF("MQTT"), 5, 0x02, 0, 60)
	body = append(body, props...)
	body = append(body, mqttUTF(id)...)
	return mqttPkt(0x10, body)
}

func mqtt5Connack(aliasMax uint16) []byte {
	if aliasMax == 0 {
		return mqttPkt(0x20, []byte{0, 0, 0})
	}
	return mqttPkt(0x20, []byte{0, 0, 3, 0x22, byte(aliasMax >> 8), byte(aliasMax)})
}

func TestProtocolSessionMQTT5ConnectPublishAndQoS(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	connect := mqtt5Connect("dev", []byte{3, 0x22, 0, 10})
	p := s.Probe(connect)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "mqtt", p.Protocol)
	require.Equal(t, "5.0", p.Version)
	r := s.Feed(0, ts, connect)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "CONNECT", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint64(10), r.Events[0].Session["Topic Alias Maximum"])
	r = s.Feed(1, ts, mqtt5Connack(10))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "CONNACK", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint64(0), r.Events[0].Session["Reason Code"])

	pub0 := mqttPkt(0x30, append(append(mqttUTF("sensors/temp"), 0), []byte("22")...))
	r = s.Feed(0, ts, pub0)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "PUBLISH", r.Events[0].Session["Packet Name"])
	require.Equal(t, "sensors/temp", r.Events[0].Session["Topic Name"])
	require.Equal(t, uint64(0), r.Events[0].Session["QoS"])

	pub1 := mqttPkt(0x32, append(append(append(mqttUTF("a/b"), 0, 1), 0), []byte("x")...))
	r = s.Feed(0, ts, pub1)
	require.Nil(t, r.Err)
	require.Equal(t, uint64(1), r.Events[0].Session["Packet Identifier"])
	require.Equal(t, "PUBACK", r.Events[0].Session["Outstanding"])
	r = s.Feed(1, ts, mqttPkt(0x40, []byte{0, 1, 0, 0}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "PUBACK", r.Events[0].Session["Packet Name"])
	require.Equal(t, true, r.Events[0].Session["Matched Request"])

	pub2 := mqttPkt(0x34, append(append(append(mqttUTF("a/b"), 0, 2), 0), []byte("y")...))
	require.Nil(t, s.Feed(0, ts, pub2).Err)
	require.Nil(t, s.Feed(1, ts, mqttPkt(0x50, []byte{0, 2})).Err)
	r = s.Feed(0, ts, mqttPkt(0x62, []byte{0, 2}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "PUBREL", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, mqttPkt(0x70, []byte{0, 2}))
	require.Nil(t, r.Err)
	require.Equal(t, "PUBCOMP", r.Events[0].Session["Packet Name"])
	require.Equal(t, true, r.Events[0].Session["Matched Request"])

	sub := mqttPkt(0x82, append(append([]byte{0, 3, 2, 0x0B, 7}, mqttUTF("a/#")...), 1))
	r = s.Feed(0, ts, sub)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SUBSCRIBE", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint64(7), r.Events[0].Session["Subscription Identifier"])
	r = s.Feed(1, ts, mqttPkt(0x90, []byte{0, 3, 0, 1}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SUBACK", r.Events[0].Session["Packet Name"])

	aliasPub := mqttPkt(0x30, append(append(mqttUTF("mapped/topic"), 3, 0x23, 0, 1), []byte("z")...))
	r = s.Feed(0, ts, aliasPub)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, uint64(1), r.Events[0].Session["Topic Alias"])
	r = s.Feed(0, ts, mqttPkt(0x30, append([]byte{0, 0, 3, 0x23, 0, 1}, []byte("z")...)))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "mapped/topic", r.Events[0].Session["Topic Name"])

	cut := s.Feed(0, ts, connect[:3])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)
}

func TestProtocolSessionMQTT5FailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, mqtt5Connect("dev", nil)).Err)
	require.Nil(t, s.Feed(1, ts, mqtt5Connack(0)).Err)
	badProp := mqttPkt(0x30, append(append(mqttUTF("t"), 2, 0xff), []byte{'x'}...))
	r := s.Feed(0, ts, badProp)
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, mqtt5Connect("dev", nil)).Err)
	require.Nil(t, s2.Feed(1, ts, mqtt5Connack(1)).Err)
	over := mqttPkt(0x30, append(append(mqttUTF("t"), 3, 0x23, 0, 2), []byte{'x'}...))
	r = s2.Feed(0, ts, over)
	require.NotNil(t, r.Err)

	truncated := mqttPkt(0x10, mqttUTF("MQTT"))
	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r = s3.Feed(0, ts, truncated[:4])
	require.True(t, r.NeedMore || r.Err != nil && r.Err.Kind == ErrNeedMore)
}

func TestProtocolSessionMQTT5Fragmentation(t *testing.T) {
	connect := mqtt5Connect("dev", nil)
	connack := mqtt5Connack(0)
	pub := mqttPkt(0x30, append(append(mqttUTF("a/b"), 0), []byte("hi")...))
	steps := []sessionStep{{0, connect}, {1, connack}, {0, pub}}
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
