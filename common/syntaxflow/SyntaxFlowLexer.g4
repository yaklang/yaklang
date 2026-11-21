lexer grammar SyntaxFlowLexer;

options {
    superClass = SyntaxFlowBaseLexer;
}


DeepFilter: '==>';
Deep: '...';

Percent: '%%';
DeepDot: '..';
// Prevent `>==` / `<==` from being tokenized as `>=` / `<=` + `=`.
// This is important for shorthand expressions like `<len>==2` where the `>` is
// a standalone token (native call terminator) followed by `==`.
LtEq: '<=' { this.GetInputStream().LA(1) != '=' }?;
GtEq: '>=' { this.GetInputStream().LA(1) != '=' }?;
DoubleGt: '>>';
Filter: '=>';
EqEq: '==';
RegexpMatch: '=~';
NotRegexpMatch: '!~';
And: '&&';
Or: '||';
NotEq: '!=';
DollarBraceOpen: '${';
Semicolon: ';';
ConditionStart: '?{';
DeepNextStart: '-{';
UseStart: '->';
DeepNextEnd: '}->';
DeepNext: '-->';
TopDefStart: '#{';
DefStart: '#>';
TopDef: '#->';
Gt: '>';
Dot: '.';
StartNowDoc: '<<<' -> pushMode(HereDocIdentifier);
Lt: '<';
Eq: '=';
Add: '+';
Amp: '&';
Question: '?';
OpenParen: '(';
Comma: ',';
CloseParen: ')';
ListSelectOpen: '[';
ListSelectClose: ']';
MapBuilderOpen: '{';
MapBuilderClose: '}';
ListStart: '#';
DollarOutput: '$';
Colon: ':';
Search: '%';
Bang: '!';
// A name filter pattern that starts with '*'. It is a separate token because a
// pattern may contain '-' (SSA value names do: Function-strcmp, if-condition)
// while a plain identifier must not: `$a-$b` is a remove reference, and letting
// '-' continue an identifier would swallow it.
// '-' is allowed inside a pattern (Function-strcmp) but only when a name
// character follows it, so `*->x`, `*-{...}->` and `*-->x` keep their original
// tokenization. The '-' is matched together with that next character because a
// lexer predicate would test LA(1) before the '-' is consumed.
GlobPattern: '*' ( '-' GlobPatternChar | GlobPatternChar )+;
Star: '*';
Minus: '-';
As: 'as';
Backtick: '`';
SingleQuote: '\'';
DoubleQuote: '"';
LineComment: '//' (~[\r\n])*;
BreakLine: '\n';
WhiteSpace: [ \r] -> skip;
Number: Digit+;
OctalNumber: '0o' OctalDigit+;
BinaryNumber: '0b' ('0' | '1')+;
HexNumber: '0x' HexDigit+;
//StringLiteral: '`' (~[`])* '`';
StringType: 'str';
ListType: 'list';
DictType: 'dict';
NumberType: 'int' | 'float';
BoolType: 'bool';
BoolLiteral: 'true' | 'false';
Alert : 'alert';
Check: 'check';
Then: 'then';
Desc: 'desc' | 'note';
Else: 'else';
Type: 'type';
In: 'in';
Call: 'call';
Function: 'function' | 'func';
Constant: 'const' | 'constant';
Phi: 'phi';
FormalParam: 'param' | 'formal_param';
Return: 'return' | 'ret';
Opcode: 'opcode';
Have: 'have';
HaveAny: 'any';
Not: 'not';
Inside: 'inside';
NotInside: 'not_inside';
For: 'for';
VersionIn: 'version_in';

ConstSearchModePrefixRegexp: 'r';
ConstSearchModePrefixGlob: 'g';
ConstSearchModePrefixExact: 'e';
Identifier: IdentifierCharStart IdentifierChar*;
IdentifierChar: [0-9] | IdentifierCharStart;

QuotedStringLiteral
    : SingleQuote ( ~['\\\r\n] | ('\\\'') | '\\\\' | '\\')* SingleQuote
    | DoubleQuote ( ~["\\\r\n] | '\\"' | '\\\\' | '\\' )* DoubleQuote
    ;



RegexpLiteral: '/' RegexpLiteralChar+ '/';


WS: [ \t\r]+ -> skip;

mode HereDocIdentifier;
HereDocIdentifierName: (NameString{this.recordHereDocLabel()}) | ('\'' (NameString{this.recordHereDocLabel()}) '\'');
CRLFHereDocIdentifierBreak: '\r\n'{this.recordHereDocLF()} -> popMode,pushMode(CRLFHereDoc);
LFHereDocIdentifierBreak: '\n'{this.recordHereDocLF()} -> popMode,pushMode(LFHereDoc);

mode CRLFHereDoc;
CRLFEndDoc:  '\r\n' NameString {this.DocEndDistribute()};
CRLFHereDocText: .;

mode LFHereDoc;
LFEndDoc:  '\n' NameString {this.DocEndDistribute()};
LFHereDocText: .;

fragment NameString: [a-zA-Z_\u0080-\ufffe][a-zA-Z0-9_\u0080-\ufffe]*;

fragment IdentifierCharStart: '*' | '_' | [a-z] | [A-Z];
fragment GlobPatternChar: [a-zA-Z0-9_*];
fragment HexDigit: [a-fA-F0-9];
fragment Digit: [0-9];
fragment OctalDigit: [0-7];
fragment RegexpLiteralChar
    : '\\' '/'
    | ~[/]
    ;
