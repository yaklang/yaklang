# scantest

Reserved for future cross-language code-scan fixtures.

## dataflow-member regression cases

`dataflow-member-<language>_test.go` pins object/member dataflow behaviour per
language, organized with `t.Run` subtests.

Each language covers the same three axes so cross-language differences are
visible in one place:

1. field/property write and read on the same object (must resolve);
2. field flowing through a real cross-function call path (must resolve);
3. independent instances with no call path between them (must NOT leak the
   field value; only the parameter itself is a legitimate resolution).

Case 3 is the guard that keeps member resolution from over-approximating: two
instances of the same type are not implicitly connected just because they share
a type name.

Small builtin-rule regression cases live in
`common/syntaxflow/sfbuildin/buildin/buildin-rule-test/`.
