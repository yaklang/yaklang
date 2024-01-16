# SSA API

> do ssa api can do anything?

the ssa api is designed for auditing and analysis, not for optimization.

optimization should be handled in Lower-IR.

## Verify Case Suite

the cases for verifying ssa api should be careful code.

u can test any language feature in ssa api;

1. rename test
2. scope test
3. if statement test
4. if statement test: phi
5. switch statement test
6. switch statement test: phi
7. loop statement test
8. loop statement test: phi
9. loop statement test: phi t2=phi(t1, t2)
10. function test
11. function test: formal parameter analysis
12. function test: the N-th formal parameter analysis
13. function test: return value analysis
14. function test: closure
15. function test: free-value
16. function test: cross
17. function test: recursive
18. function test: recursive loop phi
19. function test: maskable free-value
20. classless test: static member call
21. classless test: dynamic member call
22. classless test: static member call: phi
23. classless test: dynamic member call: phi
24. dynamic member convert to static member: collapsed literal
25. classless test and closure: multi-returns value to static member
26. function test: maskable membered free-value
27. function test: multi-returns value to value in function defs
28. classless test: container assigned in closure