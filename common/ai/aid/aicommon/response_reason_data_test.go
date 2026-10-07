package aicommon

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestAIResponseReasonDataBeforeEOF(t *testing.T) {
	for _, bound := range []bool{false, true} {
		for _, counted := range []bool{false, true} {
			name := "unbound"
			if bound {
				name = "bound"
			}
			if counted {
				name += "/counted"
			} else {
				name += "/without-consumption"
			}
			t.Run(name, func(t *testing.T) {
				response := NewAIResponse(nil)
				reader, writer := io.Pipe()
				defer writer.Close()
				defer reader.Close()
				if counted {
					response.EmitReasonStream(reader)
				} else {
					response.EmitReasonStreamWithoutConsumption(reader)
				}
				var mu sync.Mutex
				var observed strings.Builder
				first := make(chan struct{})
				completed := make(chan string, 1)
				var once sync.Once
				// Register after the provider supplies the reader, as real callers do.
				response.SetOnReasonData(func(data []byte) {
					mu.Lock()
					observed.Write(data)
					mu.Unlock()
					once.Do(func() { close(first) })
				})
				response.SetOnReasonChunk(func(data []byte) { completed <- string(data) })
				done := make(chan struct{})
				go func() {
					defer close(done)
					if bound {
						emitter := NewEmitter("reason-data-test", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) { return event, nil })
						_, _ = io.Copy(io.Discard, response.GetOutputStreamReader("test", false, emitter))
					} else {
						reason, output := response.GetUnboundStreamReaderEx(nil, nil, nil)
						_, _ = io.Copy(io.Discard, reason)
						_, _ = io.Copy(io.Discard, output)
					}
				}()
				_, err := io.WriteString(writer, "正在分析")
				require.NoError(t, err)
				select {
				case <-first:
				case <-time.After(time.Second):
					t.Fatal("Reason observation must not wait for EOF")
				}
				select {
				case <-completed:
					t.Fatal("completed Reason callback must retain its EOF semantics")
				default:
				}
				_, err = io.WriteString(writer, "，核对证据。")
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				response.Close()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("response stream did not finish")
				}
				mu.Lock()
				require.Equal(t, "正在分析，核对证据。", observed.String())
				mu.Unlock()
				require.Equal(t, "正在分析，核对证据。", response.GetPlainReason())
				if bound {
					require.Equal(t, "正在分析，核对证据。", <-completed)
				}
			})
		}
	}
}

func TestAIResponseEmptyReasonDoesNotReportData(t *testing.T) {
	response := NewAIResponse(nil)
	response.EmitReasonStream(strings.NewReader(""))
	response.EmitOutputStream(strings.NewReader("answer"))
	called := false
	response.SetOnReasonData(func([]byte) { called = true })
	response.Close()
	reason, output := response.GetUnboundStreamReaderEx(nil, nil, nil)
	_, _ = io.Copy(io.Discard, reason)
	_, _ = io.Copy(io.Discard, output)
	require.False(t, called)
}
