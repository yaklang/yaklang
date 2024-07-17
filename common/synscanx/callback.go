package synscanx

import "github.com/yaklang/yaklang/common/log"

func (s *Scannerx) OnSubmitTask(i func(addr string, port int)) {
	s.onSubmitTaskCallback = i
}

func (s *Scannerx) callOnSubmitTask(addr string, port int) {
	if s == nil {
		return
	}

	if s.onSubmitTaskCallback == nil {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("on submit task callback failed: %s", err)
		}
	}()

	s.onSubmitTaskCallback(addr, port)
}
