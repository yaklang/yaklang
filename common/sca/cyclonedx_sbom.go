package sca

import cdx "github.com/CycloneDX/cyclonedx-go"

func init() {
	bom := cdx.NewBOM()
	bom.Metadata = &cdx.Metadata{
		Timestamp:   "",
		Lifecycles:  nil,
		Tools:       nil,
		Authors:     nil,
		Component:   nil,
		Manufacture: nil,
		Supplier:    nil,
		Licenses:    nil,
		Properties:  nil,
	}
}
