parser grammar SuricataRuleParser;

options {
    tokenVocab=SuricataRuleLexer;
}

rules : rule+ EOF;
rule : action protocol src_address src_port ('->' | '<>' ) dest_address dest_port params;

action : ID;
protocol : ID;

/* parse address */
src_address: address;
dest_address : address;
address
    : 'any'
    | environment_var
    | ipv4
    | ipv6
    | '[' address (',' address)* ']'
    | '!' address
    ;
ipv4: ipv4block '.' ipv4block '.' ipv4block '.' ipv4block ('/' ipv4mask ) ? ;
ipv4block
    : INT
    ;
ipv4mask
    : INT
    ;
environment_var: '$' ID;

/* ipv6 */
ipv6
    : ( ipv6full | ipv6compact ) ( '/' ipv6mask ) ?
    ;
ipv6full
    : ipv6block ':' ipv6block ':' ipv6block ':' ipv6block ':' ipv6block ':' ipv6block ':' ipv6block ':' ipv6block
    ;
ipv6compact
    : ipv6part '::' ipv6part
    ;
ipv6part
    : ipv6block ?
    | ipv6part ':' ipv6block
    ;
ipv6block
    : HEX
    | INT
    ;
ipv6mask
    : INT
    ;

/* ports */
src_port : port;
dest_port : port;
port
    : 'any'
    | environment_var
    |  INT
    | INT ':' INT ?
    | ':' INT
    | INT ':'
    | '[' port (',' port)* ']'
    | '!' port
    ;

/* rules configuration */
params : ParamStart param (';' param) * ';'? ParamEnd;
param: ParamValue (string)?;
string: ParamQuotedString;