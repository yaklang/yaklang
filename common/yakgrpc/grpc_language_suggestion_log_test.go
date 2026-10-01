package yakgrpc

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kataras/golog"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

var languageLogAuditID atomic.Uint64

type languageLogForwarder struct {
	logger *log.Logger
}

func (w languageLogForwarder) Write(data []byte) (int, error) {
	if message := strings.TrimSpace(string(data)); message != "" {
		w.logger.Info(message)
	}
	return len(data), nil
}

func TestGRPCMUSTPASS_LANGUAGE_ConcurrentYakLoggerOutput(t *testing.T) {
	name := fmt.Sprintf("%s-%d.yak", t.Name(), languageLogAuditID.Add(1))
	logger := yaklib.CreateYakLogger(name)
	logger.SetLevel("info")
	logger.SetTimeFormat("")
	logger.SetPrefix("audit ")
	var first, second bytes.Buffer
	logger.SetOutput(&first)
	logger.SetTerminal(false)

	// Reusing a script logger must keep the caller's output destination.
	// Initializing the same logger used to overwrite a live Printer and race
	// stdout redirection while silently discarding this configuration.
	reused := yaklib.CreateYakLogger(name)
	require.Same(t, logger.Logger, reused.Logger)
	reused.Info("baseline")
	prefix := "audit " + golog.GetTextForLevel(log.InfoLevel, false) + " "
	messagePrefix := "[" + strings.TrimSuffix(name, ".yak") + "] "
	require.Equal(t, prefix+messagePrefix+"baseline\n", first.String())
	first.Reset()

	logger.SetTimeFormat("2006-01-02")
	logger.SetTerminal(true)
	logger.Info("colored")
	require.Regexp(t, "^"+regexp.QuoteMeta("audit "+golog.GetTextForLevel(log.InfoLevel, true)+" ")+`\d{4}-\d{2}-\d{2} `+regexp.QuoteMeta(messagePrefix+"colored\n")+"$", first.String())
	first.Reset()
	logger.SetTimeFormat("")

	const writers, messages = 8, 32
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(writers + 1)
	for worker := 0; worker < writers; worker++ {
		go func(worker int) {
			defer workers.Done()
			<-start
			for message := 0; message < messages; message++ {
				yaklib.CreateYakLogger(name).Infof("route:%d:%d", worker, message)
			}
		}(worker)
	}
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < messages*writers; i++ {
			if i%2 == 0 {
				logger.SetOutput(&second)
			} else {
				logger.SetOutput(&first)
			}
			logger.SetTerminal(i%2 == 0)
		}
	}()
	close(start)
	workers.Wait()

	lines := strings.Split(strings.TrimSuffix(first.String()+second.String(), "\n"), "\n")
	require.Len(t, lines, writers*messages, "switching outputs must not lose or split messages")
	seen := make(map[string]bool, writers*messages)
	for _, line := range lines {
		parts := strings.SplitN(line, messagePrefix, 2)
		require.Len(t, parts, 2, "script prefix must remain intact")
		require.True(t, parts[0] == prefix || parts[0] == "audit "+golog.GetTextForLevel(log.InfoLevel, true)+" ", "level and color formatting must remain intact: %q", parts[0])
		require.False(t, seen[parts[1]], "message duplicated: %q", parts[1])
		seen[parts[1]] = true
	}
	for worker := 0; worker < writers; worker++ {
		for message := 0; message < messages; message++ {
			require.True(t, seen[fmt.Sprintf("route:%d:%d", worker, message)])
		}
	}

	// Writers may forward to another logger, as log.NewLogWriter does. Locking
	// every logger's output with one global mutex would deadlock this path.
	var forwarded bytes.Buffer
	sink := yaklib.CreateYakLogger(name + "-sink")
	sink.SetLevel("info")
	sink.SetOutput(&forwarded)
	logger.SetOutput(languageLogForwarder{logger: sink.Logger})
	done := make(chan struct{})
	go func() {
		logger.Info("forwarded-record")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("forwarding a record to another logger deadlocked")
	}
	require.Contains(t, forwarded.String(), "forwarded-record")
	require.Equal(t, 1, strings.Count(forwarded.String(), "forwarded-record"))
}
