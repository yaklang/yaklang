# HIR

Include 25+ Instruction

| Instruction    | Description                                                                                                                        |
|----------------|------------------------------------------------------------------------------------------------------------------------------------|
| `Assert`       | assert stmt                                                                                                                        |
| `BasicBlock`   | block with scope                                                                                                                   |
| `BinOp`        | binary operation                                                                                                                   |
| `Call`         | function call or undefined call, operation with Method/FormalParams and Returns                                                    |
| `ConstInst`    | constant value / literal                                                                                                           |
| `ErrorHandler` | error handler                                                                                                                      |
| `Field`        | field access, member call, like `foo.bar`, Op1 is foo, Op2 is bar                                                                  |
| `Function`     | function definition, with FormalParams and Returns, support freevalues, closure function                                           |
| `If`           | Standard if statement, with condition, then block, else block, if-elif-else like                                                   |
| `Jump`         | jump to another block                                                                                                              |
| `Loop`         | loop statement, with condition, body block, and optional init block, and latch, classic `for` three block format                   |
| `Make`         | make statement, with type, and optional init block, create new value or mix type                                                   |
| `Next`         | next statement, with optional value, like `continue`                                                                               |
| `Panic`        | panic statement, with optional value, like `panic`                                                                                 |
| `Parameter`    | Formal Parameters, in function definitions                                                                                         | 
| `Phi`          | phi node, with type, and optional init block, like `a = phi [b, c]`, generally if-phi and for-phi is common...                     |
| `Recover`      | relative for `Panic`, with optional value, like `recover`                                                                          |
| `Return`       | return statement, with optional value, like `return`                                                                               |
| `SideEffect`   | a freevalue in function is re-assigned, like `a = 1; c = () => {a = a + 1}; c(); dump(a)` will cause the last `a` is changed       |
| `Switch`       | switch statement, with condition, and optional default block, and optional case blocks, like `switch a { case b: c; default: d; }` |
| `TypeCast`     | type cast, with type, and optional init block                                                                                      |
| `TypeValue`    | type literal, like `make([]int, 1)`, the `[]int` is type literal                                                                   |
| `UnOp`         | unary operation                                                                                                                    |
| `Undefined`    | undefined value, like `undefined`                                                                                                  |
| `Update`       | update statement, with type, and optional init block, like `a = 1; a = a + 1` will cause the last `a` is changed                   |

## How Translate From AST?

the core operator is `*Function` instance, so keep the main in a package and u can use `*Function` to `emit` some ast
structure to ssa ins.

in AST walker, the visitor mode is recommended, **DO NOT USE listener**

## Advanced Syntax Support

### Closure Function and Side Effect

```yaklang

var a = 0
c = () => { a += 1 }
c()

dump(a) // build/emit side effect via the lexical scope(name) 
```

### Lexical Scope API

```php
// in php code
<?php 

$a ="b";
$b = "123";
echo $$a; // operator for lexical scope
```

### Yield Abstract Syntax

TBD

### OOP, Interface with Object Blueprint

TBD