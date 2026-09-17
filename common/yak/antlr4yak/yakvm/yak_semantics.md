# Yak boundary contracts protected by this change

This is a focused compatibility record, not a complete language specification.
The existing language regression suite remains authoritative outside these cases.

- Indexing requires a valid element; negative indices count from the end.
  Slicing permits empty containers and empty ranges at `len`. Explicit bounds
  outside the valid range are rejected rather than clipped.
- Slice start, end and step omission are independent. Default step is one;
  negative step defaults to `len-1` through the virtual end `-1`. Explicit `-1`
  still denotes the last actual element. `a[0:0:-1]` is empty, `a[0::-1]` starts
  at the first element, and step zero fails. Extreme steps cannot overflow the
  loop index. Array slices produce `[]T`; slicing copies elements and allocates
  capacity proportional to the result. String indexing/slicing remains rune based.
- Channel sends preserve Go assignability, dynamic interface types and container
  identity; they do not apply FFI numeric normalization or container conversion.
  Nil values may be sent to nilable element types, not to numeric elements.
  Nil/full/unbuffered sends observe context
  cancellation. Cancellation already observed before select prevents a send;
  concurrent readiness permits either select outcome. Closed-channel sends and
  receive-only channels retain explicit errors. Top-level cancellation remains
  `context.Canceled` or `context.DeadlineExceeded`, as for receive/range.
- Arity checking remains enabled by default. Disabling it fills absent fixed Yak
  parameters with undefined; a variadic tail contains only unconsumed positional
  arguments. Named fixed bindings do not consume positional tail elements.
- Nil/undefined at any literal position cannot yield a nil reflection type.
  Nonempty native interfaces require implementation; named strings/bytes and
  array destinations are converted to their declared types. Existing numeric
  normalization is unchanged and is not specified here.
- Map iteration snapshots the original keys and orders their display strings.
  A key deleted before its turn is skipped; new keys are not added to that
  iteration. Equal display strings retain the existing unspecified tie order.
  This does not make concurrent script map mutation safe.
- Scope inspection snapshots binding pointers under each scope lock. It is
  neither a deep copy of user values nor an atomic snapshot across all scopes.
- Binary operands preserve left/right order. `OpAssign` and `OpFastAssign` keep
  their distinct stack order; multiple assignment, compound assignment, closures,
  defer, debug, Trace/Inline and template immutability remain regression-covered.
- `go 1+1` evaluates the entire expression in an implicit asynchronous function.
  A direct function definition, such as `go func(){...}` or `go ()=>...`, is
  implicitly invoked once; parentheses do not change this classification.
  `go factory()` invokes only factory, even if its result is a closure. Function
  values obtained through variables, members, indexing or conditional expressions
  are not implicitly invoked. `go f(args)` preserves call-site argument evaluation;
  nested calls in general expressions execute inside the implicit worker body.
  Existing instance-code syntax remains supported, and expression results are
  discarded without leaving values on the submitting frame's operand stack.
- `defer` retains its call-expression constraint (and existing panic/recover forms).
  Unknown compiler panics fail compilation and publish no code.
  Failed compile attempts do not recycle previously reserved symbol IDs.
