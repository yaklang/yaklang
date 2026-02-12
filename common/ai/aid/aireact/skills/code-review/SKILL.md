---
name: code-review
description: >
  Comprehensive code review skill covering security auditing, code quality,
  performance optimization, and engineering best practices. Guides the AI
  through a structured review process to identify vulnerabilities, anti-patterns,
  and improvement opportunities in source code.
---

# Code Review Skill

Perform a systematic code review following the checklist below.
Prioritize findings by severity: **Critical > High > Medium > Low > Info**.

---

## 1. Security Audit

### 1.1 Injection Flaws

- **SQL Injection**: Verify all database queries use parameterized statements or
  prepared queries. Flag any string concatenation that builds SQL.
- **Command Injection**: Check that user input never reaches `os/exec`, `system()`,
  or shell invocations without proper sanitization.
- **Path Traversal**: Ensure file paths derived from user input are validated
  against a whitelist or canonicalized before use. Watch for `../` sequences.
- **LDAP / XPath / Template Injection**: Identify any dynamic query or template
  construction from untrusted data.

### 1.2 Cross-Site Scripting (XSS)

- Confirm all user-supplied data rendered in HTML is context-aware escaped
  (HTML body, attribute, JavaScript, URL contexts differ).
- Check that Content-Security-Policy headers are set where applicable.

### 1.3 Authentication & Authorization

- Verify password hashing uses a modern algorithm (bcrypt, scrypt, argon2).
- Check that session tokens are generated with a cryptographically secure PRNG.
- Ensure authorization checks are performed on every privileged operation,
  not only at the UI layer.
- Flag any hard-coded credentials, API keys, or secrets.

### 1.4 Cryptography

- Verify use of up-to-date algorithms (AES-256, RSA-2048+, SHA-256+).
  Flag MD5 and SHA-1 for integrity-sensitive use.
- Ensure random values for nonces, IVs, and salts come from `crypto/rand`
  (or platform equivalent), not `math/rand`.
- Check TLS configuration: minimum TLS 1.2, no weak cipher suites.

### 1.5 Data Exposure

- Ensure sensitive data (PII, tokens, passwords) is never logged.
- Verify error messages returned to users do not leak stack traces,
  internal paths, or database schema details.
- Check that debug endpoints and verbose logging are disabled in production.

### 1.6 Deserialization

- Flag any deserialization of untrusted data (JSON is generally safe;
  native binary serialization formats like Java ObjectInputStream,
  Python pickle, PHP unserialize are high-risk).
- Verify type whitelists or integrity checks are in place.

---

## 2. Code Quality

### 2.1 Naming & Readability

- Variables, functions, and types should have descriptive, unambiguous names.
- Avoid single-letter names outside of short loop indices or well-known idioms.
- Public API names should be self-documenting; internal helpers may be terser.

### 2.2 Error Handling

- Every error returned by a function call must be checked or explicitly ignored
  with a comment explaining why.
- Avoid swallowing errors silently (`_ = fn()` without justification).
- Prefer wrapping errors with context (`fmt.Errorf("operation X: %w", err)`)
  so callers can diagnose failures.
- Panic/recover should be reserved for truly unrecoverable situations.

### 2.3 Logging

- Log messages should be in English, structured, and include relevant context
  (request ID, user ID, operation name).
- Use appropriate log levels: Debug for development, Info for normal operations,
  Warn for recoverable issues, Error for failures requiring attention.
- Never log sensitive data (passwords, tokens, full credit card numbers).

### 2.4 Code Structure

- Functions should do one thing and be short enough to reason about.
- Avoid deep nesting (> 3 levels). Prefer early returns and guard clauses.
- Group related declarations; separate concerns into distinct packages/modules.

---

## 3. Performance

### 3.1 Algorithmic Complexity

- Flag O(n^2) or worse algorithms operating on potentially large data sets.
- Suggest appropriate data structures (hash maps for lookups, heaps for
  priority queues, etc.).

### 3.2 Resource Management

- Verify that opened files, network connections, database handles, and locks
  are always released (use `defer`, `try-with-resources`, `using`, or equivalent).
- Check for goroutine/thread leaks: every launched goroutine must have
  a clear termination path.
- Ensure context cancellation is propagated to long-running operations.

### 3.3 Concurrency Safety

- Shared mutable state must be protected by a mutex, channel, or atomic operation.
- Verify there are no data races (suggest running the race detector where applicable).
- Check for potential deadlocks: consistent lock ordering, no nested locks
  without careful analysis.

### 3.4 Caching & I/O

- Identify repeated expensive computations or I/O that could be cached.
- Verify cache invalidation strategy is correct and documented.
- Check that network requests have reasonable timeouts set.

---

## 4. Best Practices

### 4.1 DRY (Don't Repeat Yourself)

- Flag duplicated logic that should be extracted into a shared function.
- Identify magic numbers or strings that should be named constants.

### 4.2 SOLID Principles

- **Single Responsibility**: Each module/class/function should have one reason to change.
- **Open-Closed**: Prefer extension over modification (interfaces, plugins).
- **Dependency Inversion**: High-level modules should depend on abstractions,
  not concrete implementations.

### 4.3 Testability

- Verify that new or modified code has corresponding tests.
- Check that dependencies are injectable (interfaces, not concrete types)
  to allow unit testing in isolation.
- Ensure test assertions are specific and will catch regressions.

### 4.4 API Design

- Public APIs should be minimal, consistent, and hard to misuse.
- Breaking changes must be clearly documented and versioned.
- Backward compatibility should be preserved unless explicitly intended otherwise.

---

## 5. Review Output Format

Summarize findings as a structured list:

```
### [Severity] Category: Brief Title

**File**: path/to/file.go:42
**Issue**: Description of the problem.
**Suggestion**: How to fix or improve.
```

End the review with a summary table counting findings by severity.
