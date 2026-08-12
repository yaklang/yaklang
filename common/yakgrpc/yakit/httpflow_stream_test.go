package yakit

import (
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func TestHTTPFlowStreamRecorderPersistsBeforeEOF(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	requestRaw := lowhttp.FixHTTPRequest([]byte("GET /events HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	req, err := lowhttp.ParseBytesToHttpRequest(requestRaw)
	require.NoError(t, err)
	httpctx.SetBareRequestBytes(req, requestRaw)
	httpctx.SetPlainRequestBytes(req, requestRaw)
	httpctx.SetRequestURL(req, "https://example.com/events")

	header := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n")
	rsp := &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Request: req,
	}
	recorder, err := NewHTTPFlowStreamRecorder(db, true, req, rsp, header)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = recorder.Close()
		_ = os.Remove(recorder.HeaderFile())
		_ = os.Remove(recorder.BodyFile())
	})

	var count int
	require.Eventually(t, func() bool {
		return db.Model(&schema.HTTPFlow{}).Where("path = ?", "/events").Count(&count).Error == nil && count == 1
	}, 2*time.Second, 20*time.Millisecond, "the header-first flow must exist before the stream reaches EOF")

	initial, err := GetHTTPFlow(db, int64(recorder.FlowID()))
	require.NoError(t, err)
	require.Equal(t, int64(200), initial.StatusCode)
	require.Equal(t, "text/event-stream", initial.ContentType)
	require.True(t, initial.IsReadTooSlowResponse)
	require.Equal(t, string(header), initial.GetResponse())

	body := []byte("d\r\ndata: ready\n\n\r\n")
	n, err := recorder.Write(body)
	require.NoError(t, err)
	require.Equal(t, len(body), n)
	require.Eventually(t, func() bool {
		flow, err := GetHTTPFlow(db, int64(recorder.FlowID()))
		return err == nil && flow.BodyLength == int64(len(body))
	}, 2*time.Second, 50*time.Millisecond)

	storedBody, err := os.ReadFile(recorder.BodyFile())
	require.NoError(t, err)
	require.Equal(t, body, storedBody)

	finalFlow, err := CreateHTTPFlowFromHTTPWithNoRspSaved(true, req, "mitm", "https://example.com/events", "127.0.0.1:443")
	require.NoError(t, err)
	finalFlow.StatusCode = 200
	finalFlow.ContentType = "text/event-stream"
	require.NoError(t, recorder.Finalize(finalFlow))

	require.NoError(t, db.Model(&schema.HTTPFlow{}).Where("path = ?", "/events").Count(&count).Error)
	require.Equal(t, 1, count, "finalization must update the header-first flow instead of inserting a duplicate")
	final, err := GetHTTPFlow(db, int64(recorder.FlowID()))
	require.NoError(t, err)
	require.Equal(t, int64(len(body)), final.BodyLength)
	require.Equal(t, recorder.HeaderFile(), final.TooLargeResponseHeaderFile)
	require.Equal(t, recorder.BodyFile(), final.TooLargeResponseBodyFile)

	packet, err := LoadHTTPFlowResponsePacket(final)
	require.NoError(t, err)
	require.Equal(t, append(header, body...), packet)
}

func TestHTTPFlowStreamRecorderDoesNotBlockOnInitialInsert(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	requestRaw := lowhttp.FixHTTPRequest([]byte("GET /events HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	req, err := lowhttp.ParseBytesToHttpRequest(requestRaw)
	require.NoError(t, err)
	httpctx.SetBareRequestBytes(req, requestRaw)
	httpctx.SetPlainRequestBytes(req, requestRaw)
	httpctx.SetRequestURL(req, "https://example.com/events")

	header := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n")
	rsp := &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Request: req,
	}
	insertStarted := make(chan struct{})
	releaseInsert := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseInsert) }) }
	t.Cleanup(release)
	recorder, err := newHTTPFlowStreamRecorder(db, true, req, rsp, header, func(db *gorm.DB, flow *schema.HTTPFlow) error {
		close(insertStarted)
		<-releaseInsert
		return InsertHTTPFlow(db, flow)
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = recorder.Close()
		_ = os.Remove(recorder.HeaderFile())
		_ = os.Remove(recorder.BodyFile())
	})

	select {
	case <-insertStarted:
	case <-time.After(time.Second):
		t.Fatal("initial insert did not start")
	}

	body := []byte("data: ready\n\n")
	writeDone := make(chan error, 1)
	go func() {
		_, writeErr := recorder.Write(body)
		writeDone <- writeErr
	}()
	select {
	case writeErr := <-writeDone:
		require.NoError(t, writeErr)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("stream body write waited for the initial database insert")
	}

	storedBody, err := os.ReadFile(recorder.BodyFile())
	require.NoError(t, err)
	require.Equal(t, body, storedBody)
	release()
	require.NotZero(t, recorder.FlowID())
}

func TestHTTPFlowStreamRecorderDropRemovesFlowAndSpillFiles(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	requestRaw := lowhttp.FixHTTPRequest([]byte("GET /events HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	req, err := lowhttp.ParseBytesToHttpRequest(requestRaw)
	require.NoError(t, err)
	httpctx.SetBareRequestBytes(req, requestRaw)
	httpctx.SetPlainRequestBytes(req, requestRaw)
	httpctx.SetRequestURL(req, "https://example.com/events")

	header := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n")
	rsp := &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Request: req,
	}
	recorder, err := NewHTTPFlowStreamRecorder(db, true, req, rsp, header)
	require.NoError(t, err)
	headerFile := recorder.HeaderFile()
	bodyFile := recorder.BodyFile()
	flowID := recorder.FlowID()
	require.NotZero(t, flowID)

	_, err = recorder.Write([]byte("data: ready\n\n"))
	require.NoError(t, err)
	require.NoError(t, recorder.Drop())

	var count int
	require.NoError(t, db.Model(&schema.HTTPFlow{}).Where("id = ?", flowID).Count(&count).Error)
	require.Zero(t, count)
	_, err = os.Stat(headerFile)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(bodyFile)
	require.True(t, os.IsNotExist(err))
}

func TestHTTPFlowStreamRecorderInsertFailureDoesNotBreakCapture(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	requestRaw := lowhttp.FixHTTPRequest([]byte("GET /events HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	req, err := lowhttp.ParseBytesToHttpRequest(requestRaw)
	require.NoError(t, err)
	httpctx.SetBareRequestBytes(req, requestRaw)
	httpctx.SetPlainRequestBytes(req, requestRaw)
	httpctx.SetRequestURL(req, "https://example.com/events")

	header := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n")
	rsp := &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Request: req,
	}
	recorder, err := newHTTPFlowStreamRecorder(db, true, req, rsp, header, func(*gorm.DB, *schema.HTTPFlow) error {
		return utils.Error("insert unavailable")
	})
	require.NoError(t, err)
	headerFile := recorder.HeaderFile()
	bodyFile := recorder.BodyFile()

	body := []byte("data: still-captured\n\n")
	n, err := recorder.Write(body)
	require.NoError(t, err)
	require.Equal(t, len(body), n)
	storedBody, err := os.ReadFile(bodyFile)
	require.NoError(t, err)
	require.Equal(t, body, storedBody)

	require.Error(t, recorder.Drop())
	_, err = os.Stat(headerFile)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(bodyFile)
	require.True(t, os.IsNotExist(err))
}
