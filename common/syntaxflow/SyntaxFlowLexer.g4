lexer grammar SyntaxFlowLexer;

options {
    superClass = SyntaxFlowBaseLexer;
}


DeepFilter: '==>';
Deep: '...';

Percent: '%%';
DeepDot: '..';
LtEq: '<=';
GtEq: '>=';
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
Function: 'function';
Constant: 'const' | 'constant';
Phi: 'phi';
FormalParam: 'param' | 'formal_param';
Return: 'return' | 'ret';
Opcode: 'opcode';
Have: 'have';
HaveAny: 'any';
Not: 'not';
For: 'for';

Identifier: IdentifierCharStart IdentifierChar*;
IdentifierChar: [0-9] | IdentifierCharStart;

QuotedStringLiteral
    : SingleQuote ( ~['\\\r\n] | ('\\\'') | '\\\\' | '\\')* SingleQuote
    | DoubleQuote ( ~["\\\r\n] | '\\"' | '\\\\' | '\\' )* DoubleQuote
    ;



RegexpLiteral: '/' RegexpLiteralChar+ '/';


WS: [ \t\r]+ -> skip;

mode HereDocIdentifier;
HereDocIdentifierName: (NameString{this.recordHereDocIdentifier()}) | ('\'' (NameString{this.recordHereDocLabel()}) '\'');
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
fragment HexDigit: [a-fA-F0-9];
fragment Digit: [0-9];
fragment OctalDigit: [0-7];
fragment RegexpLiteralChar
    : '\\' '/'
    | ~[/]
    ;