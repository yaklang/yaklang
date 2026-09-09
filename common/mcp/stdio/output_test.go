package stdio

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestProtocolWriterBlockedFile(t *testing.T) {
	for _, cancelWrite := range []bool{true, false} {
		name := "timeout"
		if cancelWrite {
			name = "cancel"
		}
		t.Run(name, func(t *testing.T) {
			reader, original, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			defer original.Close()
			// Match an inherited CLI descriptor: Fd puts a Unix os.Pipe back
			// into blocking mode. prepareOutput must make its copy cancellable.
			_ = original.Fd()
			output, cleanup, err := prepareOutput(original)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			observed := &observedWriteCloser{WriteCloser: output, entered: make(chan struct{})}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			writer := &protocolWriter{ctx: ctx, output: observed}
			done := make(chan error, 1)
			go func() {
				_, err := writer.Write(bytes.Repeat([]byte("x"), 16<<20))
				done <- err
			}()
			<-observed.entered
			wantErr := error(os.ErrDeadlineExceeded)
			limit := outputWriteTimeout + 3*time.Second
			if cancelWrite {
				cancel()
				wantErr = context.Canceled
				limit = 3 * time.Second
			}
			select {
			case err := <-done:
				if !errors.Is(err, wantErr) {
					t.Fatalf("expected %v, got %v", wantErr, err)
				}
			case <-time.After(limit):
				reader.Close() // release even a broken blocking implementation
				<-done
				t.Fatal("blocked file write did not finish")
			}
			if _, err := writer.Write([]byte("must not append another response\n")); !errors.Is(err, wantErr) {
				t.Fatalf("failed output accepted a later frame: %v", err)
			}
			if _, err := original.Stat(); err != nil {
				t.Fatalf("closed caller's descriptor, preventing CLI restoration: %v", err)
			}
		})
	}
}
