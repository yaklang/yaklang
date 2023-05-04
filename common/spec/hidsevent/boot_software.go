package hidsevent

type BootSoftware struct {
	Exe  string `exe"`
	Name string `json:"name"`
}

type BootSoftwareInfo struct {
	Software []BootSoftware `json:"software"`
}
