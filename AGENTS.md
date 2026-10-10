# Engineering Requirements

- Keep code clean, focused, and easy to read. Reuse existing helpers and remove
  duplication or dead code introduced by a change.
- Keep comments minimal. Explain non-obvious constraints, safety invariants, or
  reasons for a design choice; do not narrate code that already explains itself.
  Preserve required license notices, generated markers, and public API contracts.
- Keep architecture orderly: give each package a clear responsibility, preserve
  dependency direction, and keep protocol, persistence, and execution boundaries
  explicit. Avoid speculative abstractions and configuration.
- Prefer the smallest complete implementation that preserves correctness,
  compatibility, and meaningful regression coverage.
