package synscanx

func (s *Scannerx) OnSubmitTask(i func(addr string, port int)) {
	s.onSubmitTaskCallback = i
}
