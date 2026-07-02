:: using antlr4-tool to generate parser
del /Q /S .\parser

java -jar ..\yak\antlr4thirdparty\antlr-4.13.2-complete.jar -Dlanguage=Go .\SuricataRuleLexer.g4 .\SuricataRuleParser.g4 -o parser -no-listener -visitor