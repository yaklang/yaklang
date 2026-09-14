package aicommon

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAIResponse_ClosedStreamReadiness(t *testing.T) {
	for _, kind := range []string{"empty", "output", "reason"} {
		t.Run(kind, func(t *testing.T) {
			// Fast, already-closed responses can finish before the reader starts
			// waiting. Exercise both the first-byte and empty-response paths.
			var wg sync.WaitGroup
			for worker := 0; worker < 64; worker++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for iteration := 0; iteration < 32; iteration++ {
						response := NewUnboundAIResponse()
						var wantReason, wantOutput string
						switch kind {
						case "output":
							wantOutput = "response content"
							response.EmitOutputStream(strings.NewReader(wantOutput))
						case "reason":
							wantReason = "response reasoning"
							response.EmitReasonStream(strings.NewReader(wantReason))
						}
						response.Close()
						reasonReader, outputReader := response.GetUnboundStreamReaderEx(nil, nil, nil)
						reason, reasonErr := io.ReadAll(reasonReader)
						output, outputErr := io.ReadAll(outputReader)
						if reasonErr != nil || outputErr != nil || string(reason) != wantReason || string(output) != wantOutput {
							t.Errorf("unexpected streams: reason=%q (%v), output=%q (%v)", reason, reasonErr, output, outputErr)
							return
						}
					}
				}()
			}
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("closed AI responses must unblock their stream readers")
			}
		})
	}
}
