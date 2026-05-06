// Package syntaxflow_actions registers shared ReAct [LoopAction] factories for SyntaxFlow /
// SSA-risk loops. Handlers delegate to [syntaxflow_services]; loops should depend on this
// package rather than importing each other's loop packages to resolve actions.
//
// Dependency direction: loop/orchestrator -> syntaxflow_actions -> syntaxflow_services.
package syntaxflow_actions
