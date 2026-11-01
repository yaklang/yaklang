package pcaputil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
)

func tcpQUICCandidateVectors() []struct {
	name string
	wire []byte
} {
	return []struct {
		name string
		wire []byte
	}{
		{"version-negotiation-shaped", append([]byte{0x80, 0, 0, 0, 0, 0, 0}, make([]byte, 44)...)},
		{"RFC9001-Initial", rfc9001ClientInitial()},
	}
}

func requireNoUDPOnlyQUICOnTCP(t testing.TB, events []*BinParserEvent) {
	t.Helper()
	for _, e := range events {
		require.NotContains(t, []string{"quic", "http3", "doq"}, e.Protocol, "captured TCP bytes cannot establish a UDP-only native carrier")
	}
}

func TestQUICCapturedTCPDoesNotSelectUDPOnlySession(t *testing.T) {
	for _, candidate := range tcpQUICCandidateVectors() {
		name, wire := candidate.name, candidate.wire
		require.Equal(t, ProbeAccept, probeQUIC(wire, 4096).Verdict, "a plausible QUIC envelope still needs the correct carrier")
		for _, mode := range []string{"whole", "split", "out-of-order-retransmit", "missing-tail"} {
			steps := []tcpStep{{seq: 99, syn: true}}
			switch mode {
			case "whole":
				steps = append(steps, tcpStep{seq: 100, data: string(wire)})
			case "split":
				for at := 0; at < len(wire); at += 17 {
					steps = append(steps, tcpStep{seq: 100 + uint32(at), data: string(wire[at:min(at+17, len(wire))])})
				}
			case "out-of-order-retransmit":
				steps = append(steps, tcpStep{seq: 117, data: string(wire[17:])}, tcpStep{seq: 100, data: string(wire[:17])}, tcpStep{seq: 117, data: string(wire[17:])})
			case "missing-tail":
				steps = append(steps, tcpStep{seq: 100, data: string(wire[:len(wire)-7])})
			}
			steps = append(steps, tcpStep{seq: 100 + uint32(len(wire)), fin: true})
			capture := binTestPcap(t, steps, 1978, false, true)
			for _, workers := range []int{1, 2} {
				for _, observer := range []bool{false, true} {
					for _, deferred := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/%s/w%d/observer%v/deferred%v", name, mode, workers, observer, deferred), func(t *testing.T) {
							options := []CaptureOption{WithProtocolDeferred(deferred)}
							if observer {
								options = append(options, WithEveryPacket(func(gopacket.Packet) {}))
							}
							events, stats, err := binReplay(t, capture, workers, options...)
							if mode == "missing-tail" {
								require.ErrorContains(t, err, "unfilled sequence gap", "the absent bytes remain an explicit reassembly loss")
							} else {
								require.NoError(t, err)
							}
							requireNoUDPOnlyQUICOnTCP(t, events)
							require.Zero(t, stats.BufferedBytes)
						})
					}
				}
			}
		}
	}
}

func TestQUICLookingTCPDoesNotPoisonReusedHTTPGeneration(t *testing.T) {
	wire := rfc9001ClientInitial()
	request := "GET /after-candidate HTTP/1.1\r\nHost: example.invalid\r\n\r\n"
	response := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"
	steps := []tcpStep{
		{seq: 99, syn: true}, {seq: 499, ack: 100, syn: true, synack: true, reverse: true}, {seq: 100, ack: 500},
		{seq: 100, ack: 500, data: string(wire)}, {seq: 100 + uint32(len(wire)), ack: 500, fin: true}, {seq: 500, ack: 101 + uint32(len(wire)), fin: true, reverse: true},
		{seq: 9000, syn: true}, {seq: 12000, ack: 9001, syn: true, synack: true, reverse: true}, {seq: 9001, ack: 12001},
		{seq: 9001, ack: 12001, data: request}, {seq: 12001, ack: 9001 + uint32(len(request)), reverse: true, data: response},
		{seq: 9001 + uint32(len(request)), ack: 12001 + uint32(len(response)), fin: true}, {seq: 12001 + uint32(len(response)), ack: 9002 + uint32(len(request)), fin: true, reverse: true},
	}
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprint("deferred", deferred), func(t *testing.T) {
			events, stats, err := binReplay(t, binTestPcap(t, steps, 443, false, true), 2, WithProtocolDeferred(deferred))
			require.NoError(t, err)
			requireNoUDPOnlyQUICOnTCP(t, events)
			var http []*BinParserEvent
			for _, e := range events {
				if e.Protocol == "http" {
					http = append(http, e)
				}
			}
			require.Len(t, http, 2)
			require.Equal(t, []byte(request), http[0].Raw)
			require.Equal(t, []byte(response), http[1].Raw)
			require.Equal(t, http[0].FlowID, http[1].FlowID)
			require.Equal(t, http[0].ID, http[1].ResponseTo)
			require.Zero(t, stats.BufferedBytes)
		})
	}
}

func TestQUICNativeUDPCarrierStillAuthenticatesInitial(t *testing.T) {
	wire := rfc9001ClientInitial()
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprint("deferred", deferred), func(t *testing.T) {
			capture := sessionDatagramPCAP(t, []sessionStep{{0, wire}}, 14443)
			events, _ := discoveryReplay(t, capture, 2, true, deferred)
			require.Len(t, events, 1)
			e := events[0]
			require.Equal(t, "udp", e.Transport)
			require.Equal(t, "quic", e.Protocol)
			require.Equal(t, true, e.Session["Authentication Verified"])
			require.Equal(t, true, e.Session["Native Carrier"])
			require.Equal(t, wire, e.Raw)
		})
	}
}

func FuzzQUICCannotBecomeCapturedTCPIdentity(f *testing.F) {
	for _, wire := range tcpQUICCandidateVectors() {
		f.Add(wire.wire)
	}
	f.Add([]byte{0x40, 0, 1, 2, 3, 4, 5, 6})
	f.Fuzz(func(t *testing.T, wire []byte) {
		if len(wire) == 0 || len(wire) > 2048 {
			return
		}
		capture := binTestPcap(t, []tcpStep{{seq: 99, syn: true}, {seq: 100, data: string(bytes.Clone(wire))}, {seq: 100 + uint32(len(wire)), fin: true}}, 443, false, false)
		events, _, err := binReplay(t, capture, 1)
		require.NoError(t, err)
		requireNoUDPOnlyQUICOnTCP(t, events)
	})
}
