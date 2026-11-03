# Risk Review Report 格式

```markdown
# Risk Review Report

## 1. Review Scope

- Mode: read-only / apply
- Attached risks: 237
- Reviewed: 120
- Unreviewed: 117
- Runtime verification attempted: 31

## 2. Duplicate Groups

### Group 1

- Canonical: 101
- Duplicates: 152, 204
- Shared target: /orders
- Risk type: IDOR
- Parameter: orderId
- Evidence: evidence/risk-101-request.http

## 3. False Positives

### Risk 318

- Original claim: ...
- Original request: evidence/risk-318-request.http
- Verification request: evidence/risk-318-verification-request.http
- Verification response: evidence/risk-318-verification-response.http
- Observation: ...
- Reason for false-positive decision: ...

## 4. Confirmed Risks

- Risk ID
- Evidence
- Why it remains valid

## 5. Inconclusive

- Risk ID
- Blocker
- Required next step

## 6. Database Changes

- duplicate: 2
- false_positive: 1
- restored active: 0
- ignored: 0

## 7. Evidence Index

- path
- related risk IDs
- description
```

报告中禁止使用“应该”“可能”“看起来”作为已确认误报的唯一理由。
