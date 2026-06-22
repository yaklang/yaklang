package openapi

import "fmt"

func sanitizeParsedDocument(doc *ParsedDocument) {
	if doc == nil {
		return
	}
	for i := range doc.Operations {
		op := &doc.Operations[i]
		opRef := formatOperationRef(op.Method, op.Path)

		validParams := make([]ParameterSummary, 0, len(op.Parameters))
		for _, p := range op.Parameters {
			if IsValidParameterIn(p.In, doc.IsSwaggerV2) {
				validParams = append(validParams, p)
				continue
			}
			doc.Warnings = append(doc.Warnings, fmt.Sprintf(
				"%s: skipped parameter %q with non-standard location %q",
				opRef, p.Name, p.In,
			))
		}
		op.Parameters = validParams

		validResponses := make([]ResponseSummary, 0, len(op.Responses))
		for _, r := range op.Responses {
			if IsValidResponseStatusCode(r.StatusCode) {
				validResponses = append(validResponses, r)
				continue
			}
			doc.Warnings = append(doc.Warnings, fmt.Sprintf(
				"%s: skipped non-standard response key %q (expected HTTP status code or \"default\")",
				opRef, r.StatusCode,
			))
		}
		op.Responses = validResponses
	}
}
