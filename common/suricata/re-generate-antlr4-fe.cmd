:: using antlr4-tool to generate parser
del /Q /S .\parser

..\yak\antlr4util\antlr4.bat -Dlanguage=Go .\SuricataRuleLexer.g4 .\SuricataRuleParser.g4 -o parser -no-listener -visitor
