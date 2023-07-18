:: using antlr4-tool to generate parser
del /Q /S .\parser

antlr4 -v 4.11.1 -Dlanguage=Go .\SuricataRuleLexer.g4 .\SuricataRuleParser.g4 -o parser -no-listener -visitor