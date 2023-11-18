package yakgrpc

func (s *Server) hybridScanPause(taskId string) error {
	manager, err := GetHybridTask(taskId)
	if err != nil {
		return err
	}
	manager.Pause()
	return nil
}
