/*
Copyright (c) 2018 Javier Mena

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/
lexer grammar FreemarkerLexer;

// CHANNELS:
// 1. comments
// 2. ignored spaces in expressions

// STARTING GRAMMAR RULES
COMMENT             : COMMENT_FRAG -> channel(1);
START_DIRECTIVE_TAG : '<#' -> pushMode(EXPR_MODE);
END_DIRECTIVE_TAG   : '</#' -> pushMode(EXPR_MODE);
START_USER_DIR_TAG  : '<@' -> pushMode(EXPR_MODE);
END_USER_DIR_TAG    : '</@' -> pushMode(EXPR_MODE);
INLINE_EXPR_START   : '${' -> pushMode(EXPR_MODE);
CONTENT             : ('<' | '$' | ~[$<]+) ;

// MODES
mode DOUBLE_QUOTE_STRING_MODE;
DQS_EXIT       : '"' -> popMode;
DQS_ESCAPE     : '\\' [\\"'$n];
DQS_ENTER_EXPR : '${' -> pushMode(EXPR_MODE);
DQS_CONTENT    : (~[\\$"])+;

mode SINGLE_QUOTE_STRING_MODE;
SQS_EXIT       : '\'' -> popMode;
SQS_ESCAPE     : '\\' [\\"'$n];
SQS_ENTER_EXPR : '${' -> pushMode(EXPR_MODE);
SQS_CONTENT    : (~[\\$'])+;

mode EXPR_MODE;
// Keywords
EXPR_IF               : 'if';
EXPR_ELSE             : 'else';
EXPR_ELSEIF           : 'elseif';
EXPR_ASSIGN           : 'assign';
EXPR_AS               : 'as';
EXPR_LIST             : 'list';
EXPR_TRUE             : 'true';
EXPR_FALSE            : 'false';
EXPR_INCLUDE          : 'include';
EXPR_IMPORT           : 'import';
EXPR_MACRO            : 'macro';
EXPR_NESTED           : 'nested';
EXPR_RETURN           : 'return';
// Other symbols
EXPR_LT_SYM           : '<';
EXPR_LT_STR           : 'lt';
EXPR_LTE_SYM          : '<=';
EXPR_LTE_STR          : 'lte';
// EXPR_GT_SYM           : '>'; // Unsupported. Already defined as EXPR_EXIT_GT
EXPR_GT_STR           : 'gt';
EXPR_GTE_SYM          : '>=';
EXPR_GTE_STR          : 'gte';
EXPR_NUM              : NUMBER;
EXPR_EXIT_R_BRACE     : '}' -> popMode;
EXPR_EXIT_GT          : '>' -> popMode;
EXPR_EXIT_DIV_GT      : '/>' -> popMode;
EXPR_WS               : [ \n]+ -> channel(2);
EXPR_COMENT           : COMMENT_FRAG -> channel(1);
EXPR_STRUCT           : '{'+ -> pushMode(EXPR_MODE);
EXPR_DOUBLE_STR_START : '"' -> pushMode(DOUBLE_QUOTE_STRING_MODE);
EXPR_SINGLE_STR_START : '\'' -> pushMode(SINGLE_QUOTE_STRING_MODE);
EXPR_AT               : '@';
EXPR_DBL_QUESTION     : '??';
EXPR_QUESTION         : '?';
EXPR_BANG             : '!';
EXPR_ADD              : '+';
EXPR_SUB              : '-';
EXPR_MUL              : '*';
EXPR_DIV              : '/';
EXPR_MOD              : '%';
EXPR_L_PAREN          : '(';
EXPR_R_PAREN          : ')';
EXPR_L_SQ_PAREN       : '[';
EXPR_R_SQ_PAREN       : ']';
EXPR_COMPARE_EQ       : '==';
EXPR_EQ               : '=';
EXPR_COMPARE_NEQ      : '!=';
EXPR_LOGICAL_AND      : '&&';
EXPR_LOGICAL_OR       : '||';
EXPR_DOT              : '.';
EXPR_COMMA            : ',';
EXPR_COLON            : ':';
EXPR_SEMICOLON        : ';';
EXPR_SYMBOL           : SYMBOL;

// FRAGMENTS
fragment COMMENT_FRAG : '<#--' .*? '-->';
fragment NUMBER       : [0-9]+ ('.' [0-9]* )?;
fragment SYMBOL       : [_a-zA-Z][_a-zA-Z0-9]*;
