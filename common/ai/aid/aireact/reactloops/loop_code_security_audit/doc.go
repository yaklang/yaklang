// Package loop_code_security_audit implements the four-phase code security audit ReAct loop:
// reconnaissance → structured scan → per-finding verification → report generation.
//
// Subpackages:
//   - orchestrator: pipeline wiring across phases
//   - phase2: category scan loops and guards
//   - phase3: sequential finding verification
//   - phase4: report generation
//   - followup: post-audit Q&A mode
//   - internal/model: shared state and domain types
//   - internal/persist: workdir snapshot load/save
//   - internal/emit: structured UI events
//   - internal/util: small shared helpers
package loop_code_security_audit
