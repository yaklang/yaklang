package asyncexecmanager

// Metadata is opaque to the execution manager. An adapter may use Attributes
// for a tool name, task id or other provenance; none is interpreted here.
type Metadata struct {
	Name       string            `json:"name"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

func cloneMetadata(metadata Metadata) Metadata {
	if metadata.Attributes != nil {
		attributes := make(map[string]string, len(metadata.Attributes))
		for key, value := range metadata.Attributes {
			attributes[key] = value
		}
		metadata.Attributes = attributes
	}
	return metadata
}
