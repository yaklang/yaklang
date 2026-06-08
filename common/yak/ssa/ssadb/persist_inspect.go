package ssadb

import "database/sql/driver"

// LargeIrCodeFieldSampleBytes triggers sampled logging before persist sanitize.
const LargeIrCodeFieldSampleBytes = 64 << 20

const maxLargeFieldLogsPerBatch = 5

type irCodeFieldSize struct {
	Name string
	Len  int
}

// MeasureLargeIrCodeFields returns bind-relevant fields whose serialized size exceeds threshold.
func MeasureLargeIrCodeFields(ir *IrCode, threshold int) []irCodeFieldSize {
	if ir == nil || threshold <= 0 {
		return nil
	}
	var out []irCodeFieldSize
	appendText := func(name, text string) {
		if len(text) > threshold {
			out = append(out, irCodeFieldSize{Name: name, Len: len(text)})
		}
	}
	appendValuer := func(name string, v driver.Valuer) {
		if v == nil {
			return
		}
		raw, err := v.Value()
		if err != nil {
			return
		}
		switch t := raw.(type) {
		case string:
			appendText(name, t)
		case []byte:
			appendText(name, string(t))
		}
	}

	for _, f := range [...]struct {
		name string
		text string
	}{
		{"String", ir.String},
		{"ReadableName", ir.ReadableName},
		{"ReadableNameShort", ir.ReadableNameShort},
		{"VerboseName", ir.VerboseName},
		{"ShortVerboseName", ir.ShortVerboseName},
		{"Name", ir.Name},
		{"ExtraInformation", ir.ExtraInformation},
		{"OpcodeOperator", ir.OpcodeOperator},
		{"OpcodeName", ir.OpcodeName},
		{"SourceCodeHash", ir.SourceCodeHash},
		{"ConstType", ir.ConstType},
		{"ProgramCompileHash", ir.ProgramCompileHash},
	} {
		appendText(f.name, f.text)
	}
	for _, f := range [...]struct {
		name string
		v    driver.Valuer
	}{
		{"FormalArgs", ir.FormalArgs},
		{"FreeValues", ir.FreeValues},
		{"MemberCallArgs", ir.MemberCallArgs},
		{"SideEffects", ir.SideEffects},
		{"ReturnCodes", ir.ReturnCodes},
		{"CodeBlocks", ir.CodeBlocks},
		{"ChildrenFunction", ir.ChildrenFunction},
		{"PredBlock", ir.PredBlock},
		{"SuccBlock", ir.SuccBlock},
		{"Phis", ir.Phis},
		{"Users", ir.Users},
		{"Occulatation", ir.Occulatation},
		{"MaskedCodes", ir.MaskedCodes},
		{"Pointer", ir.Pointer},
		{"Variable", ir.Variable},
		{"ObjectMembers", ir.ObjectMembers},
	} {
		appendValuer(f.name, f.v)
	}
	return out
}

// LogLargeIrCodeFieldsSample logs code_id + field + len for oversized fields.
func LogLargeIrCodeFieldsSample(ir *IrCode, programName string) bool {
	fields := MeasureLargeIrCodeFields(ir, LargeIrCodeFieldSampleBytes)
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		log.Warnf(
			"[ssa-ir-persist] oversized field sample: program=%s code_id=%d opcode=%s field=%s len=%d threshold=%d",
			programName,
			ir.CodeID,
			ir.OpcodeName,
			f.Name,
			f.Len,
			LargeIrCodeFieldSampleBytes,
		)
	}
	return true
}

// PrepareIrCodeForPersist samples oversized fields (up to maxLargeFieldLogsPerBatch
// per call) then clamps bind payloads for SQLite.
func PrepareIrCodeForPersist(ir *IrCode, programName string, loggedSamples *int) {
	if ir == nil {
		return
	}
	if loggedSamples != nil && *loggedSamples < maxLargeFieldLogsPerBatch {
		if LogLargeIrCodeFieldsSample(ir, programName) {
			*loggedSamples++
		}
	}
	SanitizeIrCodeForPersist(ir)
}
