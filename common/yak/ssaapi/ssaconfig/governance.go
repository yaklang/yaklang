package ssaconfig

type DetectionTargetConfig struct {
	Enabled       bool    `json:"enabled,omitempty"`
	TemplateName  string  `json:"template_name,omitempty"`
	CriticalLimit int     `json:"critical_limit,omitempty"`
	HighLimit     int     `json:"high_limit,omitempty"`
	MediumLimit   int     `json:"medium_limit,omitempty"`
	LowLimit      int     `json:"low_limit,omitempty"`
	DensityLimit  float64 `json:"density_limit,omitempty"`
}

type PermissionConfig struct {
	Scope         string   `json:"scope,omitempty"`
	Members       []string `json:"members,omitempty"`
	Departments   []string `json:"departments,omitempty"`
	ManageActions []string `json:"manage_actions,omitempty"`
}

type IssueTrackerConfig struct {
	Enabled    bool   `json:"enabled,omitempty"`
	Kind       string `json:"kind,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	ProjectKey string `json:"project_key,omitempty"`
	IssueType  string `json:"issue_type,omitempty"`
	UserName   string `json:"username,omitempty"`
	Token      string `json:"token,omitempty"`
}
