#!/bin/sh

rm ./parser/*.tokens
rm ./parser/*.interp

antlr -Dlanguage=Go ./SuricataRuleLexer.g4 ./SuricataRuleParser.g4 -o parser -no-listener -visitor
