package hidsevent

type Software struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	SoftwareTimestamp int64  `json:"timestamp"`
}

type SoftwareInfo struct {
	SoftwareMgrType HIDSSoftwareType `json:"software_mgr_type"`
	InstallInfo     []Software       `json:"install_info"`
	RemoveInfo      []Software       `json:"remove_info"`
}
