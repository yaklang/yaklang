package yak

func (c *HotPatchPhaseContext) beginRequestPhaseGroup() {
	if c == nil {
		return
	}
	c.Stopped = false
	c.RetryRequested = false
	c.ClientResponse = nil
}

func (c *HotPatchPhaseContext) beginArchivePhaseGroup() {
	if c == nil {
		return
	}
	c.Stopped = false
	c.RetryRequested = false
	c.ClientResponse = nil
	c.ArchiveSkipped = false
	c.ArchiveResponse = nil
}
