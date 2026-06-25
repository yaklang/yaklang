package store

import "strings"

// VerifiedHttpApiHasProbeEvidence reports whether a verified_http_apis row reflects an actual HTTP probe.
func VerifiedHttpApiHasProbeEvidence(v *VerifiedHttpApi) bool {
	if v == nil {
		return false
	}
	if v.ProbeStatusCode != 0 {
		return true
	}
	s := strings.TrimSpace(v.ProbeAttemptsJSON)
	return s != "" && s != "null" && s != "[]"
}

// MergeVerifiedHttpApiUpdate merges an incoming upsert onto an existing row without clobbering probe evidence.
func MergeVerifiedHttpApiUpdate(existing, incoming *VerifiedHttpApi) *VerifiedHttpApi {
	if incoming == nil {
		return existing
	}
	if existing == nil {
		return incoming
	}
	out := *incoming
	out.ID = existing.ID
	out.CreatedAt = existing.CreatedAt

	existingHasProbe := VerifiedHttpApiHasProbeEvidence(existing)
	incomingHasProbe := VerifiedHttpApiHasProbeEvidence(&out)

	if existing.Verified && !out.Verified && !incomingHasProbe {
		out.Verified = existing.Verified
	}
	if existing.ProbeStatusCode != 0 && out.ProbeStatusCode == 0 {
		out.ProbeStatusCode = existing.ProbeStatusCode
	}
	if strings.TrimSpace(out.FullSampleURL) == "" {
		out.FullSampleURL = existing.FullSampleURL
	}
	if !incomingHasProbe && existingHasProbe {
		out.ProbeAttemptsJSON = existing.ProbeAttemptsJSON
		if strings.TrimSpace(out.ContentType) == "" {
			out.ContentType = existing.ContentType
		}
		if strings.TrimSpace(out.ResponseExcerpt) == "" {
			out.ResponseExcerpt = existing.ResponseExcerpt
		}
	}
	if strings.TrimSpace(out.VerdictReason) == "" {
		out.VerdictReason = existing.VerdictReason
	}
	if out.Confidence == 0 && existing.Confidence > 0 && !incomingHasProbe {
		out.Confidence = existing.Confidence
	}
	if out.Verified && existing.Verified {
		out.RejectReason = ""
	} else if strings.TrimSpace(out.RejectReason) == "" && !incomingHasProbe && strings.TrimSpace(existing.RejectReason) != "" {
		out.RejectReason = existing.RejectReason
	}
	if strings.TrimSpace(out.HandlerFile) == "" {
		out.HandlerFile = existing.HandlerFile
	}
	if strings.TrimSpace(out.HandlerSymbol) == "" {
		out.HandlerSymbol = existing.HandlerSymbol
	}
	if strings.TrimSpace(out.CodeSnippet) == "" {
		out.CodeSnippet = existing.CodeSnippet
	}
	if strings.TrimSpace(out.QueryParamsJSON) == "" {
		out.QueryParamsJSON = existing.QueryParamsJSON
	}
	if strings.TrimSpace(out.BodyHintJSON) == "" {
		out.BodyHintJSON = existing.BodyHintJSON
	}
	if strings.TrimSpace(out.AuthHeadersJSON) == "" {
		out.AuthHeadersJSON = existing.AuthHeadersJSON
	}
	return &out
}
