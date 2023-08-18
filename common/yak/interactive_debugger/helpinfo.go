package debugger

var HelpInfo = `
n, next                        : step next
in                             : step into function
out                            : step out function
r, run                         : run until breakpoint
watch <expr>                   : set observe breakpoint that is triggered when <expr> is modified
unwatch <expr>                 : remove <expr> observe breakpoint
swatch                         : show all observe breakpoints
obs <expr>                     : observe <expr>
unobs <expr>                   : un-observe <expr>
showobs                        : show all observe expressions
b, break <line> [if condition] : set normal/conditional breakpoint in line <line>
clear [line]                   : clear breakpoint in line <line> or clear all breakpoints
enable [line]                  : enable breakpoint in line <line> or enable all breakpoints
disable [line]                 : disable breakpoint in line <line> or disable all breakpoints
p, print [varname]             : print <varname> value or print all stack value
eval <expr>                    : eval expression
l, list [linenum]              : display the source code around current line or <linenum>
la                             : display all source code
so [linenum]                   : show current line opcodes
sao                            : show all opcodes
stack                          : show stack trace
h, help                        : show help info
exit                           : exit debugger

`
