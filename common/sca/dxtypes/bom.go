package dxtypes

type SCABom struct {
	// Packages is a list of packages that are direct dependencies of the
	// scanned application.
	Packages []*Package `json:"packages"`
	Services []*Service `json:"services,omitempty"`
}
