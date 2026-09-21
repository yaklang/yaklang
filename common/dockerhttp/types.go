package dockerhttp

// --- System ---

// PingInfo is returned by Ping (from headers / JSON).
type PingInfo struct {
	APIVersion   string
	OSType       string
	Experimental bool
}

// VersionInfo is returned by /version.
type VersionInfo struct {
	Version       string `json:"Version"`
	APIVersion    string `json:"ApiVersion"`
	MinAPIVersion string `json:"MinAPIVersion"`
	GitCommit     string `json:"GitCommit"`
	GoVersion     string `json:"GoVersion"`
	Os            string `json:"Os"`
	Arch          string `json:"Arch"`
	KernelVersion string `json:"KernelVersion"`
	Experimental  bool   `json:"Experimental"`
	BuildTime     string `json:"BuildTime"`
}

// --- Images ---

// ImageSummary is a list entry from GET /images/json.
type ImageSummary struct {
	ID          string            `json:"Id"`
	RepoTags    []string          `json:"RepoTags"`
	RepoDigests []string          `json:"RepoDigests"`
	Created     int64             `json:"Created"`
	Size        int64             `json:"Size"`
	Labels      map[string]string `json:"Labels"`
}

// ImageInspect is a subset of GET /images/{name}/json.
type ImageInspect struct {
	ID           string       `json:"Id"`
	RepoTags     []string     `json:"RepoTags"`
	RepoDigests  []string     `json:"RepoDigests"`
	Created      string       `json:"Created"`
	Size         int64        `json:"Size"`
	Architecture string       `json:"Architecture"`
	Os           string       `json:"Os"`
	Config       *ImageConfig `json:"Config"`
}

// ImageConfig is a minimal image config.
type ImageConfig struct {
	Env    []string          `json:"Env"`
	Cmd    []string          `json:"Cmd"`
	Labels map[string]string `json:"Labels"`
}

// JSONMessage is a progress/error event from streaming endpoints (load/pull).
type JSONMessage struct {
	Status   string     `json:"status"`
	Error    *JSONError `json:"errorDetail"`
	ErrorMsg string     `json:"error"` // legacy field
	Stream   string     `json:"stream"`
	ID       string     `json:"id"`
}

// JSONError is the nested error detail in a stream event.
type JSONError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (m *JSONMessage) Err() error {
	if m == nil {
		return nil
	}
	if m.Error != nil && m.Error.Message != "" {
		return &APIError{StatusCode: m.Error.Code, Message: m.Error.Message}
	}
	if m.ErrorMsg != "" {
		return &APIError{Message: m.ErrorMsg}
	}
	return nil
}

// --- Containers ---

// ContainerSummary is a list entry from GET /containers/json.
type ContainerSummary struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	ImageID string            `json:"ImageID"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Labels  map[string]string `json:"Labels"`
}

// ContainerInspect is a subset of GET /containers/{id}/json.
type ContainerInspect struct {
	ID              string           `json:"Id"`
	Name            string           `json:"Name"`
	Created         string           `json:"Created"`
	State           *ContainerState  `json:"State"`
	Image           string           `json:"Image"`
	Config          *ContainerConfig `json:"Config"`
	HostConfig      *HostConfig      `json:"HostConfig"`
	NetworkSettings *NetworkSettings `json:"NetworkSettings"`
	Mounts          []MountPoint     `json:"Mounts"`
}

// ContainerState holds runtime state.
type ContainerState struct {
	Status     string `json:"Status"`
	Running    bool   `json:"Running"`
	Paused     bool   `json:"Paused"`
	Restarting bool   `json:"Restarting"`
	OOMKilled  bool   `json:"OOMKilled"`
	Dead       bool   `json:"Dead"`
	Pid        int    `json:"Pid"`
	ExitCode   int    `json:"ExitCode"`
	Error      string `json:"Error"`
	StartedAt  string `json:"StartedAt"`
	FinishedAt string `json:"FinishedAt"`
}

// ContainerConfig is the portable container config used in create/inspect.
type ContainerConfig struct {
	Hostname     string              `json:"Hostname,omitempty"`
	Domainname   string              `json:"Domainname,omitempty"`
	User         string              `json:"User,omitempty"`
	AttachStdin  bool                `json:"AttachStdin,omitempty"`
	AttachStdout bool                `json:"AttachStdout,omitempty"`
	AttachStderr bool                `json:"AttachStderr,omitempty"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts,omitempty"`
	Tty          bool                `json:"Tty,omitempty"`
	OpenStdin    bool                `json:"OpenStdin,omitempty"`
	StdinOnce    bool                `json:"StdinOnce,omitempty"`
	Env          []string            `json:"Env,omitempty"`
	Cmd          []string            `json:"Cmd,omitempty"`
	Image        string              `json:"Image,omitempty"`
	Volumes      map[string]struct{} `json:"Volumes,omitempty"`
	WorkingDir   string              `json:"WorkingDir,omitempty"`
	Entrypoint   []string            `json:"Entrypoint,omitempty"`
	Labels       map[string]string   `json:"Labels,omitempty"`
}

// HostConfig holds host-dependent create options.
type HostConfig struct {
	// CPU shares (relative weight) — NanoCPUs is preferred for absolute.
	CPUShares int64 `json:"CpuShares,omitempty"`
	// NanoCPUs is CPU quota in units of 10^-9 CPUs (e.g. 2e9 = 2 CPUs).
	NanoCPUs int64 `json:"NanoCpus,omitempty"`
	// Memory in bytes. 0 = unlimited.
	Memory int64 `json:"Memory,omitempty"`
	// MemorySwap in bytes. -1 = unlimited swap; 0 = unset.
	MemorySwap int64 `json:"MemorySwap,omitempty"`
	// NetworkMode: bridge, host, none, container:<id>, or custom network name.
	NetworkMode string `json:"NetworkMode,omitempty"`
	// RestartPolicy controls automatic restarts.
	RestartPolicy *RestartPolicy `json:"RestartPolicy,omitempty"`
	// AutoRemove removes the container when it exits.
	AutoRemove bool `json:"AutoRemove,omitempty"`
	// Privileged gives extended privileges.
	Privileged bool `json:"Privileged,omitempty"`
	// Binds are volume binds "host:container[:mode]".
	Binds  []string `json:"Binds,omitempty"`
	Mounts []Mount  `json:"Mounts,omitempty"`
	// ExtraHosts are hostname mappings.
	ExtraHosts []string `json:"ExtraHosts,omitempty"`
	// PortBindings map container ports to host ports.
	PortBindings map[string][]PortBinding `json:"PortBindings,omitempty"`
	// Resources may be set via fields above; kept for JSON round-trip.
}

// PortBinding maps a container port to a host IP/port.
type PortBinding struct {
	HostIP   string `json:"HostIp,omitempty"`
	HostPort string `json:"HostPort,omitempty"`
}

// RestartPolicy is the container restart policy.
type RestartPolicy struct {
	Name              string `json:"Name,omitempty"` // "", "no", "always", "unless-stopped", "on-failure"
	MaximumRetryCount int    `json:"MaximumRetryCount,omitempty"`
}

// PortMap maps container port specs ("80/tcp") to host bindings.
type PortMap map[string][]PortBinding

// NetworkSettings is a minimal inspect field including published ports.
type NetworkSettings struct {
	IPAddress string                       `json:"IPAddress,omitempty"`
	Ports     PortMap                      `json:"Ports,omitempty"`
	Networks  map[string]*EndpointSettings `json:"Networks,omitempty"`
}

// EndpointSettings is per-network endpoint info.
type EndpointSettings struct {
	IPAddress string `json:"IPAddress,omitempty"`
	NetworkID string `json:"NetworkID,omitempty"`
}

// ContainerCreateRequest is the JSON body for POST /containers/create.
type ContainerCreateRequest struct {
	*ContainerConfig
	HostConfig       *HostConfig       `json:"HostConfig,omitempty"`
	NetworkingConfig *NetworkingConfig `json:"NetworkingConfig,omitempty"`
}

// NetworkingConfig holds per-network config at create time.
type NetworkingConfig struct {
	EndpointsConfig map[string]*EndpointSettings `json:"EndpointsConfig,omitempty"`
}

// ContainerCreateResponse is returned by create.
type ContainerCreateResponse struct {
	ID       string   `json:"Id"`
	Warnings []string `json:"Warnings"`
}

// ContainerCreateOptions holds query params for create.
type ContainerCreateOptions struct {
	Name string
}

// ContainerListOptions controls list filtering.
type ContainerListOptions struct {
	All     bool
	Limit   int
	Size    bool
	Filters Filters
}

// ImageListOptions controls image list filtering.
type ImageListOptions struct {
	All     bool
	Filters Filters
}

// ContainerRemoveOptions controls remove behavior.
type ContainerRemoveOptions struct {
	Force         bool
	RemoveVolumes bool
	RemoveLinks   bool
}

// ContainerStopOptions controls stop.
type ContainerStopOptions struct {
	Timeout *int // seconds; nil = daemon default
}

// ContainerKillOptions controls kill.
type ContainerKillOptions struct {
	Signal string
}

// ImageRemoveOptions controls image remove.
type ImageRemoveOptions struct {
	Force   bool
	NoPrune bool
}

// ImageTagOptions for tagging.
type ImageTagOptions struct {
	Repo string
	Tag  string
}

// Mount describes a host mount in a container creation request.
type Mount struct {
	Type     string `json:"Type,omitempty"`
	Source   string `json:"Source,omitempty"`
	Target   string `json:"Target,omitempty"`
	ReadOnly bool   `json:"ReadOnly,omitempty"`
}

// MountPoint describes a mounted filesystem reported by inspect.
type MountPoint struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}
