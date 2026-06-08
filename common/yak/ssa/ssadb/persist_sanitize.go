package ssadb

import (
	"unicode/utf8"
)

const (
	// maxSQLiteBindTextBytes is the maximum byte length safe to pass into
	// go-sqlite3 bind_text (C.int length). Values near math.MaxInt32 can
	// overflow and crash inside SQLite native code.
	maxSQLiteBindTextBytes = 16 * 1024 * 1024

	// maxIrCodeSliceEntries caps comma-joined Int64Slice / StringSlice columns
	// so a single row cannot produce multi-megabyte TEXT payloads.
	maxIrCodeSliceEntries = 65536
)

const irCodeTruncatedSuffix = "...(truncated for sqlite persist)"

// SanitizeIrCodeForPersist clamps oversized TEXT and slice fields before GORM/SQLite
// persistence. It is safe to call on nil.
func SanitizeIrCodeForPersist(ir *IrCode) {
	if ir == nil {
		return
	}

	for _, text := range []*string{
		&ir.String,
		&ir.ReadableName,
		&ir.ReadableNameShort,
		&ir.VerboseName,
		&ir.ShortVerboseName,
		&ir.Name,
		&ir.ExtraInformation,
		&ir.OpcodeOperator,
		&ir.OpcodeName,
		&ir.SourceCodeHash,
		&ir.ConstType,
		&ir.ProgramCompileHash,
	} {
		*text = truncateBindText(*text)
	}

	for _, slice := range []*Int64Slice{
		&ir.FormalArgs,
		&ir.FreeValues,
		&ir.MemberCallArgs,
		&ir.SideEffects,
		&ir.ReturnCodes,
		&ir.CodeBlocks,
		&ir.ChildrenFunction,
		&ir.PredBlock,
		&ir.SuccBlock,
		&ir.Phis,
		&ir.Users,
		&ir.Occulatation,
		&ir.MaskedCodes,
		&ir.Pointer,
	} {
		*slice = capInt64Slice(*slice, maxIrCodeSliceEntries)
	}
	ir.Variable = capStringSlice(ir.Variable, maxIrCodeSliceEntries)
	ir.ObjectMembers = capInt64Map(ir.ObjectMembers, maxIrCodeSliceEntries)
}

func truncateBindText(raw string) string {
	if raw == "" {
		return raw
	}
	maxLen := maxSQLiteBindTextBytes - len(irCodeTruncatedSuffix)
	if maxLen <= 0 {
		return irCodeTruncatedSuffix
	}
	if len(raw) <= maxLen {
		return raw
	}
	log.Warnf("IrCode text field truncated from %d to %d bytes before sqlite persist", len(raw), maxLen)
	cut := raw[:maxLen]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + irCodeTruncatedSuffix
}

func capInt64Slice(items Int64Slice, max int) Int64Slice {
	if max <= 0 || len(items) <= max {
		return items
	}
	log.Warnf("IrCode Int64Slice truncated from %d to %d entries before sqlite persist", len(items), max)
	return items[:max]
}

func capStringSlice(items StringSlice, max int) StringSlice {
	if max <= 0 || len(items) <= max {
		return items
	}
	log.Warnf("IrCode StringSlice truncated from %d to %d entries before sqlite persist", len(items), max)
	out := make(StringSlice, max)
	copy(out, items[:max])
	return out
}

func capInt64Map(items Int64Map, max int) Int64Map {
	if max <= 0 || len(items) <= max {
		return items
	}
	log.Warnf("IrCode Int64Map truncated from %d to %d entries before sqlite persist", len(items), max)
	return items[:max]
}
