package stdinsys

import (
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"sync"
	"time"
)

type StdinSys struct {
	multiwriter *dynamicMultiWriter
	m           *sync.Mutex
	mirrors     map[string]*Mirror
	multiWriter *dynamicMultiWriter
}

var stdinSys *StdinSys
var createOnce sync.Once
var started = utils.NewBool(false)

func GetStdinSys() *StdinSys {
	createOnce.Do(func() {
		stdinSys = &StdinSys{
			multiwriter: newDynamicMultiWriter(),
			m:           new(sync.Mutex),
			mirrors:     make(map[string]*Mirror),
		}
		done := make(chan struct{})
		defer close(done)
		stdinSys.init(done)
		<-done
		started.IsSet()
	})
	return stdinSys
}

func (s *StdinSys) waitInit() {
	for {
		if started.IsSet() {
			return
		}
		log.Debug("stdin-sys: Waiting for StdinSys to be initialized...")
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *StdinSys) init(start chan struct{}) {
	go func() {
		log.Info("stdin-sys: Starting to read from stdin and write to mirrors")
		var buf = make([]byte, 1)
		startedOnce := new(sync.Once)
		for {
			startedOnce.Do(func() {
				start <- struct{}{}
			})
			n, _ := os.Stdin.Read(buf)
			if n > 0 {
				_, _ = s.multiwriter.Write(buf[:n])
			}
		}
	}()
}

func (s *StdinSys) GetStdinMirror(spec string) *Mirror {
	s.m.Lock()
	defer s.m.Unlock()

	if mi, ok := s.mirrors[spec]; ok {
		return mi
	}
	return nil
}

func (s *StdinSys) GetDefaultStdinMirror() *Mirror {
	result := s.GetStdinMirror("default")
	if result == nil {
		result = s.CreateStdinMirror("default")
	}
	return result
}

func (s *StdinSys) PreventDefaultStdinMirror() {
	s.RemoveStdinMirror("default")
}

func (s *StdinSys) HaveDefaultStdinMirror() bool {
	s.m.Lock()
	defer s.m.Unlock()
	_, ok := s.mirrors["default"]
	return ok
}

func (s *StdinSys) CreateStdinMirror(spec string) *Mirror {
	s.m.Lock()
	defer s.m.Unlock()

	log.Infof("stdin-sys: start to creating a new mirror named: %s", spec)
	mi := newMirror()
	s.multiwriter.AddWriter(spec, mi)
	s.mirrors[spec] = mi
	return mi
}

func (s *StdinSys) CreateTemporaryStdinMirror() (string, *Mirror) {
	id := ksuid.New().String()
	return id, s.CreateStdinMirror(id)
}

func (s *StdinSys) RemoveStdinMirror(spec string) {
	s.m.Lock()
	defer s.m.Unlock()
	s.multiwriter.RemoveWriter(spec)
	if m, ok := s.mirrors[spec]; ok {
		m.Close()
		delete(s.mirrors, spec)
	}
}
