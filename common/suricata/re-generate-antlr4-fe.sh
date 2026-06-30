#!/bin/sh

rm ./parser/*.tokens
rm ./parser/*.interp

java -jar ../yak/antlr4thirdparty/antlr-4.13.2-complete.jar -Dlanguage=Go ./SuricataRuleLexer.g4 ./SuricataRuleParser.g4 -o parser -no-listener -visitor
