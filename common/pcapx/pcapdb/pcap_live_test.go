package pcapdb

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func liveFeed(t testing.TB, w *LiveWriter, raw []byte) {
	t.Helper()
	require.NoError(t, w.Feed(raw, gopacket.CaptureInfo{Timestamp: time.Now(), CaptureLength: len(raw), Length: len(raw)}))
}

func TestLiveCaptureQueriesAcrossCommits(t *testing.T) {
	m := testManager(t)
	w, err := m.CreateLiveDataset(context.Background(), LiveCaptureConfig{Interface: "controlled", BatchSize: 1}, layers.LinkTypeEthernet)
	require.NoError(t, err)
	defer w.Close()
	// A separate manager must open the live manifest without waiting for its
	// writer. This exercises the cold path rather than cached reader borrowing.
	other, err := NewInstanceManager(m.profile, m.root)
	require.NoError(t, err)
	defer other.Close()
	db, err := other.Open(context.Background(), w.ID)
	require.NoError(t, err)
	defer db.Close()
	empty, err := db.QueryProtocolsPage()
	require.NoError(t, err)
	require.Empty(t, empty.Items)
	require.Equal(t, StateReady, empty.State)
	liveFeed(t, w, tcpPacket(t, 100, 0, "S", nil, false))
	liveFeed(t, w, tcpPacket(t, 500, 101, "SA", nil, true))
	liveFeed(t, w, tcpPacket(t, 101, 501, "A", nil, false))
	request := []byte("GET /live-proof HTTP/1.1\r\nHost: live.example\r\n\r\n")
	liveFeed(t, w, tcpPacket(t, 101, 501, "AP", request[:12], false))
	liveFeed(t, w, tcpPacket(t, 113, 501, "AP", request[12:], false))
	page, err := db.QueryProtocolsPage(QueryProtocol("http"))
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, StateReady, page.State)
	requestID := page.Items[0].ID
	detail, err := db.ProtocolDetailsPage(requestID, QueryResultBytes(65536))
	require.NoError(t, err)
	require.Equal(t, StateReady, detail.State)
	require.NotEmpty(t, detail.Items[0].Fields)
	refs, err := db.ProtocolPacketIDs(requestID)
	require.NoError(t, err)
	require.Equal(t, []int64{4, 5}, refs)
	response := []byte("HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\nLIVE")
	liveFeed(t, w, tcpPacket(t, 501, uint32(101+len(request)), "AP", response, true))
	page, err = db.QueryProtocolsPage(QueryProtocol("http"))
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	require.EqualValues(t, requestID, page.Items[1].ResponseTo)
	body, err := db.HTTPBodyProfilePage(page.Items[1].ID)
	require.NoError(t, err)
	require.Equal(t, StateReady, body.State)
	fields, err := db.DiscoverProtocolFields(QueryProtocol("http"), QueryResultBytes(65536))
	require.NoError(t, err)
	require.NotEmpty(t, fields.Items)
	require.Equal(t, StateReady, fields.State)
	sessions, err := db.QuerySessionsPage()
	require.NoError(t, err)
	require.Len(t, sessions.Items, 1)
	require.Equal(t, "open", sessions.Items[0].CloseReason)
	streams, err := db.QueryStreamsPage(QuerySession(sessions.Items[0].ID))
	require.NoError(t, err)
	require.Len(t, streams.Items, 2)
	preview, err := db.ReadStreamPage(streams.Items[0].ID)
	require.NoError(t, err)
	require.NotEmpty(t, preview.Items)
	require.Equal(t, StateReady, preview.State)
	packets, err := db.QueryPacketsPage()
	require.NoError(t, err)
	require.Len(t, packets.Items, 6)
	require.Equal(t, StateReady, packets.State)
	packetData, err := db.ReadPacketPage(1)
	require.NoError(t, err)
	require.Equal(t, StateReady, packetData.State)
	protocolData, err := db.ReadProtocolPage(requestID)
	require.NoError(t, err)
	require.Equal(t, StateReady, protocolData.State)
	linked, err := db.PacketSessions(4)
	require.NoError(t, err)
	require.Len(t, linked, 1)
	macs, err := db.QueryMACSummariesPage()
	require.NoError(t, err)
	require.NotEmpty(t, macs.Items)
	require.Equal(t, StateReady, macs.State)
	bindings, err := db.QueryEndpointBindingsPage()
	require.NoError(t, err)
	require.NotEmpty(t, bindings.Items)
	require.Equal(t, StateReady, bindings.State)
	_, err = m.Export(context.Background(), w.ID, filepath.Join(t.TempDir(), "busy.pcap"))
	require.ErrorIs(t, err, ErrBusy)
	_, err = m.Delete(context.Background(), w.ID)
	require.ErrorIs(t, err, ErrBusy)
	require.NoError(t, w.Close())
	require.NoError(t, w.Close())
	sessions, err = db.QuerySessionsPage()
	require.NoError(t, err)
	require.Equal(t, StateReady, sessions.State)
	require.Equal(t, "capture-end", sessions.Items[0].CloseReason)
	page, err = db.QueryProtocolsPage(QueryProtocol("http"))
	require.NoError(t, err)
	require.Equal(t, requestID, page.Items[0].ID)
	valid, err := m.Validate(context.Background(), w.ID, true)
	require.NoError(t, err)
	require.True(t, valid.Valid, valid.Error)
	output := filepath.Join(t.TempDir(), "live.pcap")
	_, err = m.Export(context.Background(), w.ID, output)
	require.NoError(t, err)
	imported, err := m.GetOrCreate(output, WithProtocols(true))
	require.NoError(t, err)
	defer imported.Close()
	meta, err := imported.Metadata()
	require.NoError(t, err)
	require.EqualValues(t, 6, meta.PacketCount)
}

func TestLiveCaptureTimerCancellationAndIdenticalEmptyCaptures(t *testing.T) {
	m := testManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w, err := m.CreateLiveDataset(ctx, LiveCaptureConfig{FlushInterval: 5 * time.Millisecond}, layers.LinkTypeEthernet)
	require.NoError(t, err)
	defer w.Close()
	db, err := m.Open(context.Background(), w.ID)
	require.NoError(t, err)
	defer db.Close()
	liveFeed(t, w, dnsPacket(t))
	require.Eventually(t, func() bool { meta, err := db.Metadata(); return err == nil && meta.PacketCount == 1 }, time.Second, 5*time.Millisecond)
	cancel()
	select {
	case <-w.done:
	case <-time.After(time.Second):
		t.Fatal("capture did not finish on owner cancellation")
	}
	require.NoError(t, w.Close())
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.Equal(t, StateReady, meta.State)
	for i := 0; i < 2; i++ {
		empty, err := m.CreateLiveDataset(context.Background(), LiveCaptureConfig{}, layers.LinkTypeEthernet)
		require.NoError(t, err)
		require.NoError(t, empty.Close())
		valid, err := m.Validate(context.Background(), empty.ID, true)
		require.NoError(t, err)
		require.True(t, valid.Valid, valid.Error)
	}
}

func TestLiveCaptureConcurrentReadersAndFailedBatch(t *testing.T) {
	m := testManager(t)
	w, err := m.CreateLiveDataset(context.Background(), LiveCaptureConfig{BatchSize: 2, FlushInterval: 5 * time.Millisecond}, layers.LinkTypeEthernet)
	require.NoError(t, err)
	defer w.Close()
	db, err := m.Open(context.Background(), w.ID)
	require.NoError(t, err)
	defer db.Close()
	var wg sync.WaitGroup
	readErr := make(chan error, 1)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 15; j++ {
				overview, err := db.CaptureOverview()
				if err == nil {
					var protocols, statuses int64
					for _, count := range overview.Protocols {
						protocols += count.Count
					}
					for _, count := range overview.Statuses {
						statuses += count.Count
					}
					if protocols != statuses {
						err = errors.New("atomic DNS overview counts disagree")
					}
				}
				if err != nil {
					select {
					case readErr <- err:
					default:
					}
					return
				}
			}
		}()
	}
	for i := 0; i < 20; i++ {
		liveFeed(t, w, dnsPacket(t))
	}
	wg.Wait()
	select {
	case err := <-readErr:
		t.Fatal(err)
	default:
	}
	require.NoError(t, w.Flush())
	// Reject the next actual INSERT to exercise a failed SQL transaction. Only
	// the previous committed batch remains readable after the capture fails.
	require.NoError(t, w.writer.Exec("CREATE TRIGGER reject_live_packet BEFORE INSERT ON packets BEGIN SELECT RAISE(ABORT,'injected live write failure'); END").Error)
	raw := dnsPacket(t)
	err = w.Feed(raw, gopacket.CaptureInfo{Timestamp: time.Now(), CaptureLength: len(raw), Length: len(raw)})
	require.ErrorContains(t, err, "injected live write failure")
	require.Error(t, w.Close())
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.EqualValues(t, 20, meta.PacketCount)
	require.Equal(t, StateReady, meta.State)
	require.Contains(t, meta.LastError, "injected live write failure")
	packets, err := db.QueryPacketsPage(QueryLimit(100))
	require.NoError(t, err)
	require.Len(t, packets.Items, 20)
	require.Equal(t, StateReady, packets.State)
}

func TestLiveCaptureNativeIdleCancellation(t *testing.T) {
	device := os.Getenv("PCAPX_LIVE_TEST_IFACE")
	if device == "" {
		t.Skip("set PCAPX_LIVE_TEST_IFACE for native lifecycle validation")
	}
	m := testManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan PCAPFileDBMetadata, 1)
	done := make(chan error, 1)
	go func() {
		_, err := m.RunLiveCapture(ctx, LiveCaptureConfig{Interface: device, BPF: "udp and dst port 54321"}, func(meta PCAPFileDBMetadata) { started <- meta })
		done <- err
	}()
	select {
	case meta := <-started:
		db, err := m.Open(context.Background(), meta.DatasetID)
		require.NoError(t, err)
		_, err = db.QueryPacketsPage()
		require.NoError(t, err)
		require.NoError(t, db.Close())
	case err := <-done:
		t.Fatalf("capture startup failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("capture startup timed out")
	}
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("idle capture failed to stop")
	}
}

func TestLiveCaptureCrashRecovery(t *testing.T) {
	root := t.TempDir()
	profilePath := filepath.Join(t.TempDir(), "profile.sqlite")
	command := exec.Command(os.Args[0], "-test.run=^TestLiveCaptureCrashWorker$", "-test.count=1")
	command.Env = append(os.Environ(), "PCAPDB_LIVE_CRASH_ROOT="+root, "PCAPDB_LIVE_CRASH_PROFILE="+profilePath)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	profile, err := gorm.Open("sqlite3", profilePath)
	require.NoError(t, err)
	defer profile.Close()
	m, err := NewInstanceManager(profile, root)
	require.NoError(t, err)
	defer m.Close()
	entries, err := m.List()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, StateReady, entries[0].State)
	db, err := m.Open(context.Background(), entries[0].DatasetID)
	require.NoError(t, err)
	defer db.Close()
	packets, err := db.QueryPacketsPage()
	require.NoError(t, err)
	require.Len(t, packets.Items, 1)
	require.Equal(t, StateReady, packets.State)
}

func TestLiveCaptureCrashWorker(t *testing.T) {
	root := os.Getenv("PCAPDB_LIVE_CRASH_ROOT")
	if root == "" {
		t.Skip("subprocess crash worker")
	}
	profile, err := gorm.Open("sqlite3", os.Getenv("PCAPDB_LIVE_CRASH_PROFILE"))
	require.NoError(t, err)
	m, err := NewInstanceManager(profile, root)
	require.NoError(t, err)
	w, err := m.CreateLiveDataset(context.Background(), LiveCaptureConfig{FlushInterval: time.Minute}, layers.LinkTypeEthernet)
	require.NoError(t, err)
	liveFeed(t, w, dnsPacket(t))
	require.NoError(t, w.Flush())
	liveFeed(t, w, dnsPacket(t))
	// Deliberately bypass all cleanup to model process loss between commits.
	os.Exit(0)
}

func TestLiveCaptureManagerShutdownJoinsWriter(t *testing.T) {
	m := testManager(t)
	w, err := m.CreateLiveDataset(context.Background(), LiveCaptureConfig{}, layers.LinkTypeEthernet)
	require.NoError(t, err)
	liveFeed(t, w, dnsPacket(t))
	require.NoError(t, m.Close())
	select {
	case <-w.done:
	default:
		t.Fatal("manager returned before capture cleanup")
	}
	require.EqualValues(t, 1, w.Metadata().PacketCount)
	require.Equal(t, StateReady, w.Metadata().State)
}
