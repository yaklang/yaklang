package aspparser

import (
	"sync"
	"testing"
)

func TestSerializedATNInitializationIsConcurrentSafe(t *testing.T) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := 0; worker < 64; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if len(GetASPLexerSerializedATN()) == 0 || len(GetASPParserSerializedATN()) == 0 {
				t.Error("serialized ATN was empty after initialization")
			}
		}()
	}
	close(start)
	wg.Wait()
}
