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
    | variableAssignStatement eos
    | functionDeclarationStatement
    ;
block
    : '{' eos? statementList? '}'
    ;
variableDeclarationStatement: ( GlobalVar | LocalVar ) identifier (',' identifier)*;

variableAssignStatement: (GlobalVar | LocalVar | Var) identifier ('=' singleExpression)?;

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
    : arrayLiteral                                                           # ArrayLiteralExpression
    | singleExpression '.' Identifier                                        # MemberDotExpression
    | singleExpression '(' argumentList? ')'                                 # CallExpression
    | '(' expressionSequence ')'                                             # ParenthesizedExpression
    | singleExpression '[' singleExpression ']'                              # MemberIndexExpression
    | '++' singleExpression                                                  # PreIncrementExpression
    | '--' singleExpression                                                  # PreDecreaseExpression
    | '+' singleExpression                                                   # UnaryPlusExpression
    | '-' singleExpression                                                   # UnaryMinusExpression
    | '~' singleExpression                                                   # BitNotExpression
    | singleExpression  '++'                                                 # PostIncrementExpression
    | singleExpression  '--'                                                 # PostDecreaseExpression
    | singleExpression ( '**' | '*' | '/' | '%') singleExpression                    # MultiplicativeExpression
    | singleExpression ('+' | '-') singleExpression                          # AdditiveExpression
    | singleExpression ('<<' | '>>'  | '<<<' | '>>>' ) singleExpression                # BitShiftExpression
    | singleExpression ('<' | '>' | '<=' | '>=') singleExpression            # RelationalExpression
    | singleExpression X singleExpression                                      # XExpression
    | singleExpression ('==' | '>!<' | '><' | '!=' | '!~' | '=~') singleExpression        # EqualityExpression
    | '!' singleExpression                                                   # NotExpression // 根据gb_wmi_access.nasl第65行调整!优先级
    | singleExpression '&' singleExpression                                  # BitAndExpression
    | singleExpression '|' singleExpression                                  # BitOrExpression
    | singleExpression '^' singleExpression                                  # BitXOrExpression
    | singleExpression '&&' singleExpression                                 # LogicalAndExpression
    | singleExpression '||' singleExpression                                 # LogicalOrExpression
    | identifier (('[' singleExpression ']')|('.' identifier))? assignmentOperator singleExpression     # AssignmentExpression
    | identifier                                                             # IdentifierExpression
    | literal                                                                # LiteralExpression
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
    | '<<='
    | '>>='
    | '<<<='
    | '>>>='
    ;
eos
    : SemiColon+
    ;
