package pcaptools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
)

type captureOutput chan []byte

func (output captureOutput) Write(data []byte) (int, error) {
	output <- append([]byte(nil), data...)
	return len(data), nil
}

func TestCaptureToolIdentityQueryAndCancellation(t *testing.T) {
	profile, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "profile.sqlite"))
	require.NoError(t, err)
	defer profile.Close()
	m, err := pcapdb.NewInstanceManager(profile, t.TempDir())
	require.NoError(t, err)
	defer m.Close()
	started := make(captureOutput, 1)
	runner := func(ctx context.Context, m *pcapdb.InstanceManager, c pcapdb.LiveCaptureConfig, onStarted func(pcapdb.PCAPFileDBMetadata)) (*pcapdb.PCAPFileDBMetadata, error) {
		w, err := m.CreateLiveDataset(ctx, c, layers.LinkTypeEthernet)
		if err != nil {
			return nil, err
		}
		onStarted(w.Metadata())
		raw := make([]byte, 60)
		if err = w.Feed(raw, gopacket.CaptureInfo{Timestamp: time.Now(), CaptureLength: len(raw), Length: len(raw)}); err != nil {
			return nil, err
		}
		<-ctx.Done()
		err = w.Close()
		meta := w.Metadata()
		return &meta, err
	}
	tools := createTools(func() (*pcapdb.InstanceManager, error) { return m, nil }, runner)
	tool := tools[1]
	require.Equal(t, "start_pcap_capture", tool.Name)
	require.True(t, tool.DefaultAsync)
	valid, validationErrs := tool.ValidateParams(map[string]any{"interface": "controlled"})
	require.True(t, valid, "%v", validationErrs)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan *aitool.ToolExecutionResult, 1)
	errCh := make(chan error, 1)
	go func() {
		config := aitool.NewToolInvokeConfig()
		for _, option := range []aitool.ToolInvokeOptions{
			aitool.WithStdout(started), aitool.WithStderr(io.Discard),
			aitool.WithWaitForCallbackOnCancel(true),
		} {
			option(config)
		}
		result, err := tool.ExecuteToolWithCapture(ctx, map[string]any{"interface": "controlled"}, config)
		done <- result
		errCh <- err
	}()
	var partial map[string]any
	select {
	case raw := <-started:
		require.NoError(t, json.Unmarshal(raw, &partial))
	case <-time.After(3 * time.Second):
		t.Fatal("startup dataset identity was not written to stdout")
	}
	id := partial["dataset_id"].(string)
	db, err := m.Open(context.Background(), id)
	require.NoError(t, err)
	defer db.Close()
	require.Eventually(t, func() bool { page, err := db.QueryPacketsPage(); return err == nil && len(page.Items) == 1 }, time.Second, 5*time.Millisecond)
	select {
	case <-done:
		t.Fatal("capture tool completed while its owner remained active")
	default:
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("capture did not finish cleanup")
	}
	require.ErrorIs(t, <-errCh, context.Canceled)
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.Equal(t, pcapdb.StateReady, meta.State)
	require.Equal(t, id, meta.DatasetID)
	require.EqualValues(t, 1, meta.PacketCount)
}

func TestCaptureToolStartupFailureAndMetadata(t *testing.T) {
	tools := createTools(func() (*pcapdb.InstanceManager, error) { return nil, errors.New("unavailable library") }, nil)
	tool := tools[1]
	_, err := tool.Callback(context.Background(), aitool.InvokeParams{"interface": "controlled"}, nil, io.Discard, io.Discard)
	require.ErrorContains(t, err, "unavailable library")
	valid, _ := tool.ValidateParams(map[string]any{})
	require.False(t, valid)
}
