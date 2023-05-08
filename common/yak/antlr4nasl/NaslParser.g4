parser grammar NaslParser;

options {
  tokenVocab = NaslLexer;
}


program
    : statementList? EOF
    ;
statementList
    : statement+
    ;
statement
    : block
    | ifStatement
    | iterationStatement
    | continueStatement eos
    | breakStatement eos
    | returnStatement eos
    | expressionStatement eos
    | variableDeclarationStatement eos
    | functionDeclarationStatement
    | exitStatement eos
    ;
block
    : '{' eos? statementList? '}'
    ;
variableDeclarationStatement: ( GlobalVar | LocalVar ) identifier (',' identifier)*;
expressionStatement
    : expressionSequence
    ;

ifStatement
    : If '(' singleExpression ')' eos? statement (Else statement)?
    ;

iterationStatement
    : For '(' singleExpression? ';' singleExpression? ';' singleExpression? ')' eos? statement  #TraditionalFor
    | ForEach identifier '(' singleExpression ')' statement #ForEach
    | While '(' singleExpression ')' statement #While
    | Repeat statement Until  singleExpression  eos #Repeat
    ;

continueStatement
    : Continue
    ;

breakStatement
    : Break
    ;

returnStatement
    : Return ('(' singleExpression ')' | singleExpression)?
    ;
exitStatement
    : Exit '(' singleExpression ')'
    ;
argumentList
    : argument (',' argument)*
    ;

argument
    : (identifier ':')? singleExpression
    ;

expressionSequence
    : singleExpression (',' singleExpression)*
    ;

functionDeclarationStatement
    : Function_ identifier '(' parameterList? ')' block
    ;

parameterList
    : identifier (',' identifier)*
    ;
arrayLiteral
    : ('[' elementList? ']')
    ;

elementList
    : arrayElement (','+ arrayElement)*
    ;

arrayElement
    : (singleExpression | identifier) ','?
    ;
singleExpression
    : singleExpression '(' argumentList? ')'                                 # CallExpression

    | singleExpression '[' singleExpression ']'                              # MemberIndexExpression
    | singleExpression ( '**' | '*' | '/' | '%') singleExpression                    # MultiplicativeExpression
    | singleExpression ('+' | '-') singleExpression                          # AdditiveExpression
    | singleExpression ('<<' | '>>' | '>>>' | '>>=' | '>>>=' ) singleExpression                # BitShiftExpression
    | singleExpression ('<' | '>' | '<=' | '>=') singleExpression            # RelationalExpression
    | singleExpression ('==' | '>!<' | '><' | '!=' | '!~' | '=~') singleExpression        # EqualityExpression
    | singleExpression '&' singleExpression                                  # BitAndExpression
    | singleExpression '^' singleExpression                                  # BitXOrExpression
    | singleExpression '|' singleExpression                                  # BitOrExpression
    | identifier                                                             # IdentifierExpression
    | literal                                                                # LiteralExpression
    | arrayLiteral                                                           # ArrayLiteralExpression
    | singleExpression '.' Identifier                                        # MemberDotExpression
    | singleExpression (('[' singleExpression ']')|('.' Identifier))? assignmentOperator singleExpression     # AssignmentExpression
    | '!' singleExpression                                                   # NotExpression
    | '(' expressionSequence ')'                                             # ParenthesizedExpression
    | singleExpression X singleExpression                                      # XExpression
    | singleExpression  '++'                                                 # PostIncrementExpression
    | singleExpression  '--'                                                 # PostDecreaseExpression
    | '++' singleExpression                                                  # PreIncrementExpression
    | '--' singleExpression                                                  # PreDecreaseExpression
    | '+' singleExpression                                                   # UnaryPlusExpression
    | '-' singleExpression                                                   # UnaryMinusExpression
    | '~' singleExpression                                                   # BitNotExpression
    | singleExpression '&&' singleExpression                                 # LogicalAndExpression
    | singleExpression '||' singleExpression                                 # LogicalOrExpression

    ;
//memberDotExp:
//    singleExpression '.' identifier;

literal
    : BooleanLiteral
    | StringLiteral
    | numericLiteral
    | IpLiteral
    | NULLLiteral
    ;

numericLiteral
    : IntegerLiteral
    | FloatLiteral
    | HexLiteral
    ;
identifier
    : Identifier
    | X;
assignmentOperator
    : '*='
    | '/='
    | '%='
    | '+='
    | '-='
    | '='
    ;
eos
    : SemiColon+
    ;
