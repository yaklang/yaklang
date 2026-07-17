#!/bin/sh

rm ./parser/*.tokens
rm ./parser/*.interp

../yak/antlr4util/antlr4 -Dlanguage=Go ./SuricataRuleLexer.g4 ./SuricataRuleParser.g4 -o parser -no-listener -visitor
