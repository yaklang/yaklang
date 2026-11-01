package pcaputil

import (
	"bytes"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFreshMySQLGreetingDoesNotBecomePostgreSQL(t *testing.T) {
	wire, err := hex.DecodeString("500000000a382e302e33362d6175646974002a000000590836a3c4fbd9430005a221020008001500000000000000000000eecb1896558f345dbaf4aba8006d7973716c5f6e61746976655f70617373776f726400")
	require.NoError(t, err)
	require.Equal(t, ProbeAccept, probeMySQL(wire, 1<<20).Verdict)
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	f := &binFlow{a: c.binParser, id: 1, captureTCP: true, tcpClientKnown: true}
	f.detectDirection(1, wire)
	require.Equal(t, "mysql", f.protocol)
	require.NotNil(t, f.mysql)
}

func TestNativeMidstreamPOP3StatusDoesNotEstablishRedis(t *testing.T) {
	a, events := remainingParser(t)
	f := &binFlow{a: a, id: 1, captureTCP: true, transportFlow: &TrafficFlow{}}
	f.feed(1, []byte("+OK\r\nFrom: audit@example.invalid\r\n"), time.Unix(1, 0))
	f.close(TrafficFlowCloseReason_FIN)
	for _, e := range *events {
		require.NotEqual(t, "redis", e.Protocol)
	}
	require.Zero(t, a.stats().BufferedBytes)
	// An observed request still establishes RESP; later scalar responses inherit it.
	es, _ := sessionTestFlow(t, "redis", []sessionStep{{0, []byte("*1\r\n$4\r\nPING\r\n")}, {1, []byte("+PONG\r\n")}}, 1, false)
	require.Len(t, es, 2)
	for _, e := range es {
		require.Equal(t, "redis", e.Protocol)
		require.Equal(t, "decoded", e.Status)
	}
}

func remainingParser(t *testing.T) (*binParser, *[]*ProtocolEvent) {
	t.Helper()
	var events []*ProtocolEvent
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })(c))
	require.NoError(t, c.prepareBinParser())
	return c.binParser, &events
}

func TestFTPObservedDataAssociationBoundaries(t *testing.T) {
	for _, fault := range []string{"positive", "no-transfer", "other-peer", "other-domain", "expired", "closed-control", "ambiguous", "ambiguous-renegotiation", "ambiguous-closed-control", "protection-unacknowledged", "protection-acknowledged"} {
		t.Run(fault, func(t *testing.T) {
			a, _ := remainingParser(t)
			defer a.closeFTPEndpoints()
			control := &binFlow{a: a, id: 1, transportFlow: &TrafficFlow{}, ftp: &binFTP{}, endpoints: [2]string{"192.0.2.1:44000", "192.0.2.2:21"}}
			ts := time.Unix(1000, 0)
			observe := func(f *binFlow, dir int, wire string, values map[string]any) {
				f.observeFTPDataControl(dir, &ProtocolEvent{Timestamp: ts, Raw: []byte(wire), Session: values})
			}
			command := func(name string) {
				observe(control, 0, name+" file\r\n", map[string]any{"Role": "command", "Packet Name": name})
			}
			command("PASV")
			passive := map[string]any{"Role": "reply", "Packet Name": "Reply", "In Reply To": "PASV", "Reply Code": 227}
			observe(control, 1, "227 Entering Passive Mode (203,0,113,99,200,1)\r\n", passive)
			if fault == "ambiguous" || fault == "ambiguous-renegotiation" || fault == "ambiguous-closed-control" {
				other := &binFlow{a: a, id: 2, transportFlow: &TrafficFlow{}, ftp: &binFTP{clientKnown: true}, endpoints: control.endpoints}
				observe(other, 1, "227 Entering Passive Mode (192,0,2,2,200,1)\r\n", passive)
				if fault == "ambiguous-renegotiation" {
					observe(control, 1, "227 Entering Passive Mode (192,0,2,2,200,1)\r\n", passive)
				}
				if fault == "ambiguous-closed-control" {
					a.forgetFTPControl(control.id)
				}
			}
			if fault == "protection-unacknowledged" || fault == "protection-acknowledged" {
				observe(control, 0, "PROT P\r\n", map[string]any{"Role": "command", "Packet Name": "PROT"})
				if fault == "protection-acknowledged" {
					observe(control, 1, "200 Protected\r\n", map[string]any{"Role": "reply", "Packet Name": "Reply", "In Reply To": "PROT", "Reply Code": 200})
				}
			}
			if fault != "no-transfer" {
				command("RETR")
			}
			data := &binFlow{a: a, id: 3, endpoints: [2]string{"192.0.2.1:44001", "192.0.2.2:51201"}}
			if fault == "other-peer" {
				data.endpoints[0] = "192.0.2.3:44001"
			}
			if fault == "other-domain" {
				data.domain.Interface = 1
			}
			if fault == "expired" {
				ts = ts.Add(3 * time.Minute)
			}
			if fault == "closed-control" {
				a.forgetFTPControl(control.id)
			}
			binding := a.matchFTPData(data, ts)
			if fault == "positive" || fault == "protection-unacknowledged" || fault == "protection-acknowledged" {
				require.NotNil(t, binding)
				require.Equal(t, fault == "protection-acknowledged", binding.protected)
				require.Equal(t, "192.0.2.2:51201", binding.key.endpoint, "PASV advertised IP never overrides captured peer")
				require.Nil(t, a.matchFTPData(&binFlow{a: a, endpoints: data.endpoints}, ts), "one negotiation binds at most one data flow")
				data.closeSession()
			} else {
				require.Nil(t, binding)
			}
			a.closeFTPEndpoints()
			require.Zero(t, a.stats().BufferedBytes)
		})
	}
}

func TestFTPDataChunksHonorMessageBudgetAndNeverSniffContent(t *testing.T) {
	a, events := remainingParser(t)
	a.config.MaxMessageBytes, a.budget.MaxFrameBytes = 64, 64
	f := &binFlow{a: a, id: 1, protocol: "ftp-data", ftpData: &ftpDataBinding{transfer: 9, command: "RETR"}}
	wire := bytes.Repeat([]byte("SSH-2.0-body\r\n"), 300)
	f.feed(0, wire, time.Unix(1, 0))
	var got []byte
	for _, e := range *events {
		require.Equal(t, "ftp-data", e.Protocol)
		require.LessOrEqual(t, len(e.Raw), 64)
		require.EqualValues(t, len(got), e.Session["Stream Offset"])
		got = append(got, e.Raw...)
	}
	require.Equal(t, wire, got)
	f.closeSession()
	require.Zero(t, a.stats().BufferedBytes)
}

func TestMQTT311KeepsPayloadAndQoS2State(t *testing.T) {
	body := append(mqttUTF("MQTT"), 4, 2, 0, 30)
	body = append(body, mqttUTF("fresh")...)
	payload := []byte("\x00\xffSSH-2.0-body\r\n")
	pub := append(mqttUTF("audit/data"), 0, 7)
	pub = append(pub, payload...)
	steps := []sessionStep{{0, mqttPkt(0x10, body)}, {1, mqttPkt(0x20, []byte{0, 0})}, {0, mqttPkt(0x34, pub)}, {1, mqttPkt(0x50, []byte{0, 7})}, {0, mqttPkt(0x62, []byte{0, 7})}, {1, mqttPkt(0x70, []byte{0, 7})}}
	for _, chunk := range []int{0, 1, 7} {
		es, _ := sessionTestFlow(t, "mqtt", steps, chunk, false)
		require.Len(t, es, 6)
		for _, e := range es {
			require.Equal(t, "decoded", e.Status, e.Error)
			decoded, err := e.Decode()
			require.NoError(t, err)
			meta, ok := decoded["metadata"].(map[string]any)
			require.True(t, ok)
			require.EqualValues(t, 4, meta["Protocol Level Context"])
		}
		fields, err := es[2].GetFields()
		require.NoError(t, err)
		require.Equal(t, payload, findSessionField(fields, "Application Message"))
		for _, e := range es[3:] {
			require.Equal(t, true, e.Session["Matched Request"])
		}
	}
}

func TestSMBZeroSessionResponseMustBeUniqueAndOppositeDirection(t *testing.T) {
	for _, fault := range []string{"positive", "ambiguous", "same-direction", "cancel"} {
		t.Run(fault, func(t *testing.T) {
			s := &binSMB2{pending: map[smb2PendingKey]smb2Pending{}, async: map[smb2AsyncKey]smb2PendingKey{}, files: map[smb2FileKey]string{}}
			request := map[string]any{"Packet Name": "LOGOFF", "Message ID": uint64(9), "Session ID": uint64(42), "Tree ID": uint32(0)}
			require.NoError(t, s.associateFrom(request, 16, 0))
			if fault == "ambiguous" {
				request = cloneSession(request)
				request["Session ID"] = uint64(43)
				require.NoError(t, s.associateFrom(request, 16, 0))
			}
			if fault == "cancel" {
				cancel := cloneSession(request)
				cancel["Packet Name"] = "CANCEL"
				require.NoError(t, s.associateFrom(cancel, 16, 0))
				require.Len(t, s.pending, 1)
				require.Equal(t, false, cancel["Response Expected"])
			}
			reply := map[string]any{"Packet Name": "LOGOFF", "Message ID": uint64(9), "Session ID": uint64(0), "Tree ID": uint32(0), "Response": true, "Status": uint32(0)}
			dir := 1
			if fault == "same-direction" {
				dir = 0
			}
			require.NoError(t, s.associateFrom(reply, 16, dir))
			if fault == "ambiguous" || fault == "same-direction" {
				require.Equal(t, true, reply["Unmatched"])
				require.NotEmpty(t, s.pending)
			} else {
				require.Equal(t, "matched", reply["Association Status"])
				require.Empty(t, s.pending)
			}
		})
	}
}
