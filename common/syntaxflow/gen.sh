#!/bin/bash
antlr -Dlanguage=Go ./SyntaxFlow.g4 -o sf -package sf -no-listener -visitor
