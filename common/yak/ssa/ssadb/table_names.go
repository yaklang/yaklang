package ssadb

const (
	TableIrCodes = "ir_codes"
	// TableIrConsts is the legacy const table: one row per const instruction,
	// keyed by code_id. It is read but never written by this build, so a
	// database written before the const pool keeps working.
	TableIrConsts = "ir_consts_v1"
	// TableIrConstPool is one row per distinct constant value per program.
	// ir_codes.const_id points at it, so the text is stored once.
	TableIrConstPool = "ir_const_pool_v1"
	TableIrIndices   = "ir_indices_v1"
	TableIrNamePool  = "ir_name_pool_v1"
	TableIrSources   = "ir_sources"
	TableIrPrograms  = "ir_programs"
	TableIrOffsets   = "ir_offsets"
	TableIrTypes     = "ir_types_v2"

	TableAuditResults = "audit_results"
	TableAuditNodes   = "audit_nodes"
	TableAuditEdges   = "audit_edges"
)
