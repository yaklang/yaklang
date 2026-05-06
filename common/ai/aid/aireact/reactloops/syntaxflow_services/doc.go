// Package syntaxflow_services provides deterministic SSA / SyntaxFlow operations without depending
// on a specific ReAct loop implementation. Loops, orchestrators, and syntaxflow_actions consume
// these helpers; dependency direction is reactloops -> syntaxflow_actions -> syntaxflow_services -> yakit/schema.
//
// Subpackages are organized by concern: risk overview/review, scan session loading, project/rule/bulk (see individual .go files).
package syntaxflow_services
