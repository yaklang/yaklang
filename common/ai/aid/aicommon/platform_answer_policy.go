package aicommon

// GetFinishAfterDirectlyAnswer is a host-selected completion policy. It does not
// bypass open TODO work or allow any additional model action.
func (c *Config) GetFinishAfterDirectlyAnswer() bool { return c.FinishAfterDirectlyAnswer }
