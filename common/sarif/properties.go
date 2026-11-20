package sarif

// Properties ...
type Properties map[string]interface{}

// PropertyBag ...
type PropertyBag struct {
	Properties Properties `json:"properties,omitempty"`
}

// NewPropertyBag creates a new PropertyBag and returns a pointer to it
func NewPropertyBag() *PropertyBag {
	return &PropertyBag{
		Properties: Properties{},
	}
}
