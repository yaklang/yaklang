package parser

import (
	"slices"

	"github.com/yaklang/yaklang/common/yak/typescript/frontend/ast"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/core"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/scanner"
)

func ensureScriptKind(fileName string, scriptKind core.ScriptKind) core.ScriptKind {
	// Using scriptKind as a condition handles both:
	// - 'scriptKind' is unspecified and thus it is `undefined`
	// - 'scriptKind' is set and it is `Unknown` (0)
	// If the 'scriptKind' is 'undefined' or 'Unknown' then we attempt
	// to get the ScriptKind from the file name. If it cannot be resolved
	// from the file name then the default 'TS' script kind is returned.
	if scriptKind == core.ScriptKindUnknown {
		scriptKind = core.GetScriptKindFromFileName(fileName)
	}
	if scriptKind == core.ScriptKindUnknown {
		scriptKind = core.ScriptKindTS
	}
	return scriptKind
}

func getLanguageVariant(scriptKind core.ScriptKind) core.LanguageVariant {
	switch scriptKind {
	case core.ScriptKindTSX, core.ScriptKindJSX, core.ScriptKindJS, core.ScriptKindJSON:
		// .tsx and .jsx files are treated as jsx language variant.
		return core.LanguageVariantJSX
	}
	return core.LanguageVariantStandard
}

func tokenIsIdentifierOrKeyword(token ast.Kind) bool {
	return token >= ast.KindIdentifier
}

func tokenIsIdentifierOrKeywordOrGreaterThan(token ast.Kind) bool {
	return token == ast.KindGreaterThanToken || tokenIsIdentifierOrKeyword(token)
}

func getJSDocCommentRanges(f *ast.NodeFactory, commentRanges []ast.CommentRange, node *ast.Node, text string) []ast.CommentRange {
	switch node.Kind {
	case ast.KindParameter, ast.KindTypeParameter, ast.KindFunctionExpression, ast.KindArrowFunction, ast.KindParenthesizedExpression, ast.KindVariableDeclaration, ast.KindExportSpecifier:
		for _, commentRange := range scanner.GetTrailingCommentRanges(f, text, node.Pos()) {
			commentRanges = append(commentRanges, commentRange)
		}
		for _, commentRange := range scanner.GetLeadingCommentRanges(f, text, node.Pos()) {
			commentRanges = append(commentRanges, commentRange)
		}
	default:
		for _, commentRange := range scanner.GetLeadingCommentRanges(f, text, node.Pos()) {
			commentRanges = append(commentRanges, commentRange)
		}
	}
	// Keep if the comment starts with '/**' but not if it is '/**/'
	return slices.DeleteFunc(commentRanges, func(comment ast.CommentRange) bool {
		return comment.End() > node.End() || text[comment.Pos()+1] != '*' || text[comment.Pos()+2] != '*' || text[comment.Pos()+3] == '/'
	})
}

func isKeywordOrPunctuation(token ast.Kind) bool {
	return ast.IsKeywordKind(token) || ast.IsPunctuationKind(token)
}

func isJSDocLikeText(text string) bool {
	return len(text) >= 4 && text[1] == '*' && text[2] == '*' && text[3] != '/'
}
