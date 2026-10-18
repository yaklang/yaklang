//go:build cgo

package minirehs

import (
	"sync"
	"testing"
)

func TestNativeArenaReloadOwners(t *testing.T) {
	pats := []Pattern{{ID: 0, Expr: `a[bc]token`}, {ID: 1, Expr: `\b[0-9]{3}\b`}}
	subject := []byte("abtoken 123 actoken")
	for _, locate := range []bool{false, true} {
		compiled, err := Compile(pats, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		image, err := compiled.MarshalImage()
		compiled.Close()
		if err != nil {
			t.Fatal(err)
		}
		old, err := LoadImage(image)
		if err != nil {
			t.Fatal(err)
		}
		next, err := LoadImage(image)
		if err != nil {
			old.Close()
			t.Fatal(err)
		}
		clear(image)
		want := programOracle(pats, subject, BackendMVS, locate, 0)
		var oldWG, newWG sync.WaitGroup
		errs := make(chan error, 8)
		start := make(chan struct{})
		for i := 0; i < 8; i++ {
			db, wg := old, &oldWG
			if i%2 != 0 {
				db, wg = next, &newWG
			}
			wg.Add(1)
			go func(d Database, done *sync.WaitGroup) {
				defer done.Done()
				<-start
				for j := 0; j < 20; j++ {
					if err := programCompare(programScan(t, d, subject, 0), want, false); err != nil {
						errs <- err
						return
					}
				}
			}(db, wg)
		}
		close(start)
		oldWG.Wait() // Caller joins this DB's readers before Close.
		old.Close()
		newWG.Wait()
		close(errs)
		for err := range errs {
			t.Error(err)
		}
		if err := programCompare(programScan(t, next, subject, 0), want, false); err != nil {
			t.Error("closing old owner changed replacement", err)
		}
		next.Close()
	}
}
