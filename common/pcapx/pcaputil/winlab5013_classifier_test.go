package pcaputil

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func replayWinlab5013Protocols(t *testing.T, name string) []*ProtocolEvent {
	t.Helper()
	path := filepath.Join("..", "..", "bin-parser", "testdata", "winlab5013", "captures", name)
	var events []*ProtocolEvent
	err := ReplayPcapFile(path,
		WithTCPReassemblyWorkers(1),
		WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }),
	)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	return events
}

func TestWinlab5013SOCKS5CaptureIsNotMisclassifiedAsTNS(t *testing.T) {
	events := replayWinlab5013Protocols(t, "01-socks5.pcapng")
	var socks []*ProtocolEvent
	for _, event := range events {
		require.NotEqual(t, "tns", event.Protocol, "%s %s: %s", event.Status, event.Protocol, event.Summary)
		if event.Protocol == "socks5" {
			require.Equal(t, "decoded", event.Status)
			socks = append(socks, event)
		}
	}
	require.Len(t, socks, 4)
	require.Equal(t, "ClientNegotiation", socks[0].Entry)
	require.Equal(t, "ServerNegotiation", socks[1].Entry)
	require.Equal(t, "Request", socks[2].Entry)
	require.Equal(t, uint8(1), socks[2].Fields["Command"])
	require.Equal(t, uint16(80), socks[2].Fields["DstPort"])
	require.Equal(t, "Replies", socks[3].Entry)
	require.Equal(t, uint8(0), socks[3].Fields["Reply"])
	var tunneledHTTP int
	for _, event := range events {
		if event.Protocol == "http" && event.Status == "decoded" {
			tunneledHTTP++
		}
	}
	require.Equal(t, 2, tunneledHTTP)
}

func TestWinlab5013ZooKeeperCaptureIsRecognizedAsZooKeeper(t *testing.T) {
	var zooKeeper []*ProtocolEvent
	for _, event := range replayWinlab5013Protocols(t, "07-zookeeper.pcapng") {
		require.NotEqual(t, "kafka", event.Protocol, "%s %s: %s", event.Status, event.Protocol, event.Summary)
		if event.Protocol == "zookeeper" {
			require.Equal(t, "decoded", event.Status, "%s: %s", event.Status, event.Summary)
			zooKeeper = append(zooKeeper, event)
		}
	}
	require.Len(t, zooKeeper, 10)
	require.Equal(t, []string{
		"Connect Request", "Connect Response", "Ping", "Ping Response",
		"GetChildren", "GetChildren Response", "GetData", "GetData Response",
		"CloseSession", "CloseSession Response",
	}, []string{
		zooKeeper[0].Fields["Packet Name"].(string),
		zooKeeper[1].Fields["Packet Name"].(string),
		zooKeeper[2].Fields["Packet Name"].(string),
		zooKeeper[3].Fields["Packet Name"].(string),
		zooKeeper[4].Fields["Packet Name"].(string),
		zooKeeper[5].Fields["Packet Name"].(string),
		zooKeeper[6].Fields["Packet Name"].(string),
		zooKeeper[7].Fields["Packet Name"].(string),
		zooKeeper[8].Fields["Packet Name"].(string),
		zooKeeper[9].Fields["Packet Name"].(string),
	})
	require.Equal(t, uint64(0x5013), zooKeeper[1].Fields["Session ID"])
	require.Equal(t, []string{"lab", "znode"}, zooKeeper[5].Fields["Children"])
	require.Equal(t, "/lab", zooKeeper[7].Fields["Path"])
	require.Equal(t, "winlab", zooKeeper[7].Fields["Value"])
}

func TestWinlab5013CardDAVPROPFINDCaptureIsRecognizedAsHTTP(t *testing.T) {
	var messages []*ProtocolEvent
	for _, event := range replayWinlab5013Protocols(t, "11-carddav.pcapng") {
		if event.Protocol == "http" {
			require.Equal(t, "decoded", event.Status, "%s: %s", event.Status, event.Summary)
			messages = append(messages, event)
		}
	}
	require.Len(t, messages, 6)
	require.Equal(t, []string{
		"PROPFIND /card/alice/ HTTP/1.1",
		"HTTP/1.1 207 Multi-Status",
		"REPORT /card/alice/ HTTP/1.1",
		"HTTP/1.1 207 Multi-Status",
		"PUT /card/alice/lab-card-5013.vcf HTTP/1.1",
		"HTTP/1.1 201 Created",
	}, []string{
		messages[0].Summary,
		messages[1].Summary,
		messages[2].Summary,
		messages[3].Summary,
		messages[4].Summary,
		messages[5].Summary,
	})
}
