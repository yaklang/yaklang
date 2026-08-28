package codeaudit

// FrameworkDetection is the result of detecting a framework.
type FrameworkDetection struct {
	Name       string  `json:"name"`
	Display    string  `json:"display"`
	Confidence float64 `json:"confidence"`
}

// CmsDetection is the result of detecting a CMS product.
type CmsDetection struct {
	ID         string  `json:"id"`
	Display    string  `json:"display"`
	Family     string  `json:"family"`
	Confidence float64 `json:"confidence"`
}
