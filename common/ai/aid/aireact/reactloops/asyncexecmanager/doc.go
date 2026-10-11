// Package asyncexecmanager owns cancellable asynchronous executions and exposes
// their live inspectors. It depends only on the Go standard library: tool calls,
// agents and other resources integrate through NewResource(execute, inspect),
// or by implementing Resource and Inspector directly.
//
// The manager owns execution identity and lifecycle, while the resource owns its
// observations. List and Wait never sample or cache stdout, stderr or results.
// Inspect reads the resource's inspector on demand, including after execution
// has ended. Resources must retain their inspectable final observations.
//
// Cancellation is cooperative. Execute must not return until its work and
// cleanup have finished; Stop requests cancellation, and Wait/Shutdown confirm
// that Execute returned. A waiter timing out does not stop an execution.
package asyncexecmanager
