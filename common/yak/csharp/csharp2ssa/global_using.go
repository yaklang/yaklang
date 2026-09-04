package csharp2ssa

import (
	"strings"
	"sync"

	"github.com/yaklang/antlr/v4"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// A global using belongs to the whole C# compilation, not to the source file
// that contains it. Project files are parsed by independent singleFileBuilder
// values and their lazy bodies can run in later compile-unit batches, so the
// directives need a project-scoped, concurrency-safe registry.
type globalUsingState struct {
	mu         sync.RWMutex
	namespaces []string
	statics    []string
	aliases    map[string]string
	aliasTypes map[string]csharpparser.INamespace_or_type_nameContext
}

type globalUsingSnapshot struct {
	namespaces []string
	statics    []string
	aliases    map[string]string
	aliasTypes map[string]csharpparser.INamespace_or_type_nameContext
}

func newGlobalUsingSnapshot() globalUsingSnapshot {
	return globalUsingSnapshot{
		aliases:    make(map[string]string),
		aliasTypes: make(map[string]csharpparser.INamespace_or_type_nameContext),
	}
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func (s *globalUsingSnapshot) addNamespace(path string) {
	s.namespaces = appendUnique(s.namespaces, path)
}

func (s *globalUsingSnapshot) addStatic(path string) {
	s.statics = appendUnique(s.statics, path)
}

func (s *globalUsingSnapshot) addAlias(name, target string, node csharpparser.INamespace_or_type_nameContext) {
	name, target = strings.TrimSpace(name), strings.TrimSpace(target)
	if name == "" || target == "" {
		return
	}
	if s.aliases == nil {
		s.aliases = make(map[string]string)
	}
	if s.aliasTypes == nil {
		s.aliasTypes = make(map[string]csharpparser.INamespace_or_type_nameContext)
	}
	s.aliases[name] = target
	if node != nil {
		s.aliasTypes[name] = node
	}
}

func (s *globalUsingState) replace(snapshot globalUsingSnapshot) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.namespaces = append([]string(nil), snapshot.namespaces...)
	s.statics = append([]string(nil), snapshot.statics...)
	s.aliases = cloneUsingAliases(snapshot.aliases)
	s.aliasTypes = cloneUsingAliasTypes(snapshot.aliasTypes)
	s.mu.Unlock()
}

func (s *globalUsingState) snapshot() globalUsingSnapshot {
	if s == nil {
		return newGlobalUsingSnapshot()
	}
	s.mu.RLock()
	snapshot := globalUsingSnapshot{
		namespaces: append([]string(nil), s.namespaces...),
		statics:    append([]string(nil), s.statics...),
		aliases:    cloneUsingAliases(s.aliases),
		aliasTypes: cloneUsingAliasTypes(s.aliasTypes),
	}
	s.mu.RUnlock()
	return snapshot
}

func (s *globalUsingState) addNamespace(path string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.namespaces = appendUnique(s.namespaces, path)
	s.mu.Unlock()
}

func (s *globalUsingState) addStatic(path string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.statics = appendUnique(s.statics, path)
	s.mu.Unlock()
}

func (s *globalUsingState) addAlias(name, target string, node csharpparser.INamespace_or_type_nameContext) {
	if s == nil {
		return
	}
	name, target = strings.TrimSpace(name), strings.TrimSpace(target)
	if name == "" || target == "" {
		return
	}
	s.mu.Lock()
	if s.aliases == nil {
		s.aliases = make(map[string]string)
	}
	if s.aliasTypes == nil {
		s.aliasTypes = make(map[string]csharpparser.INamespace_or_type_nameContext)
	}
	s.aliases[name] = target
	if node != nil {
		s.aliasTypes[name] = node
	}
	s.mu.Unlock()
}

func (b *singleFileBuilder) visibleUsings() []string {
	global := b.globalUsings.snapshot()
	ret := append([]string(nil), global.namespaces...)
	for _, path := range b.usings {
		ret = appendUnique(ret, path)
	}
	return ret
}

func (b *singleFileBuilder) visibleUsingStatics() []string {
	global := b.globalUsings.snapshot()
	ret := append([]string(nil), global.statics...)
	for _, path := range b.usingStatics {
		ret = appendUnique(ret, path)
	}
	return ret
}

func (b *singleFileBuilder) lookupUsingAlias(name string) (string, csharpparser.INamespace_or_type_nameContext, bool) {
	if target, ok := b.usingAlias[name]; ok {
		return target, b.usingAliasTypes[name], true
	}
	global := b.globalUsings.snapshot()
	target, ok := global.aliases[name]
	if !ok {
		return "", nil, false
	}
	return target, global.aliasTypes[name], true
}

func (b *singleFileBuilder) hasUsingAlias(name string) bool {
	_, _, ok := b.lookupUsingAlias(name)
	return ok
}

type csharpScanToken struct {
	typ  int
	text string
}

type csharpScannedUsing struct {
	target string
	global bool
}

// csharpFileScan is the project-planning view of one source file. It is kept
// deliberately smaller than an AST: compilation-unit partitioning needs only
// the declared namespace, import targets and the global-using snippets that are
// parsed into the project-wide registry below.
type csharpFileScan struct {
	namespaceName    string
	namespaceNames   []string
	usings           []csharpScannedUsing
	globalDirectives []string
}

// csharpFileScanState avoids lexing every file once during partitioning and a
// second time during dependency extraction. PartitionCompileUnits replaces the
// whole map at the beginning of each project lifecycle; the lazy fallback also
// makes direct CompileUnitDependencies calls safe.
type csharpFileScanState struct {
	mu    sync.RWMutex
	files map[string]csharpFileScan
}

func (s *csharpFileScanState) replace(files map[string]csharpFileScan) {
	if s == nil {
		return
	}
	copyFiles := make(map[string]csharpFileScan, len(files))
	for file, scan := range files {
		copyFiles[file] = scan
	}
	s.mu.Lock()
	s.files = copyFiles
	s.mu.Unlock()
}

func (s *csharpFileScanState) load(file string) (csharpFileScan, bool) {
	if s == nil {
		return csharpFileScan{}, false
	}
	s.mu.RLock()
	scan, ok := s.files[file]
	s.mu.RUnlock()
	return scan, ok
}

func (s *csharpFileScanState) loadOrStore(file string, scan func() csharpFileScan) csharpFileScan {
	if cached, ok := s.load(file); ok {
		return cached
	}
	candidate := scan()
	s.mu.Lock()
	if s.files == nil {
		s.files = make(map[string]csharpFileScan)
	}
	if cached, ok := s.files[file]; ok {
		s.mu.Unlock()
		return cached
	}
	s.files[file] = candidate
	s.mu.Unlock()
	return candidate
}

func defaultCSharpTokens(source string) []csharpScanToken {
	lexer := newCSharpLexer(antlr.NewInputStream(source))
	raw := lexer.GetAllTokens()
	tokens := make([]csharpScanToken, 0, len(raw))
	for _, token := range raw {
		if token == nil || token.GetChannel() != antlr.TokenDefaultChannel {
			continue
		}
		tokens = append(tokens, csharpScanToken{typ: token.GetTokenType(), text: token.GetText()})
	}
	return tokens
}

func isCSharpIdentifierToken(token csharpScanToken) bool {
	switch token.typ {
	case csharpparser.CSharpLexerSimple_Identifier,
		csharpparser.CSharpLexerKW_ARGLIST,
		csharpparser.CSharpLexerKW_ADD,
		csharpparser.CSharpLexerKW_ALIAS,
		csharpparser.CSharpLexerKW_ASCENDING,
		csharpparser.CSharpLexerKW_ASYNC,
		csharpparser.CSharpLexerKW_AWAIT,
		csharpparser.CSharpLexerKW_BY,
		csharpparser.CSharpLexerKW_DESCENDING,
		csharpparser.CSharpLexerKW_DYNAMIC,
		csharpparser.CSharpLexerKW_EQUALS,
		csharpparser.CSharpLexerKW_FROM,
		csharpparser.CSharpLexerKW_GET,
		csharpparser.CSharpLexerKW_GLOBAL,
		csharpparser.CSharpLexerKW_GROUP,
		csharpparser.CSharpLexerKW_INTO,
		csharpparser.CSharpLexerKW_JOIN,
		csharpparser.CSharpLexerKW_LET,
		csharpparser.CSharpLexerKW_NAMEOF,
		csharpparser.CSharpLexerKW_NOTNULL,
		csharpparser.CSharpLexerKW_ON,
		csharpparser.CSharpLexerKW_ORDERBY,
		csharpparser.CSharpLexerKW_INIT,
		csharpparser.CSharpLexerKW_PARTIAL,
		csharpparser.CSharpLexerKW_REMOVE,
		csharpparser.CSharpLexerKW_SELECT,
		csharpparser.CSharpLexerKW_SET,
		csharpparser.CSharpLexerKW_UNMANAGED,
		csharpparser.CSharpLexerKW_VALUE,
		csharpparser.CSharpLexerKW_VAR,
		csharpparser.CSharpLexerKW_WHEN,
		csharpparser.CSharpLexerKW_WHERE,
		csharpparser.CSharpLexerKW_YIELD:
		return true
	default:
		return false
	}
}

func consumeCSharpTypeArguments(tokens []csharpScanToken, index int) (int, bool) {
	if index >= len(tokens) || tokens[index].typ != csharpparser.CSharpLexerTK_LT {
		return index, true
	}
	depth := 0
	for cursor := index; cursor < len(tokens); cursor++ {
		switch tokens[cursor].typ {
		case csharpparser.CSharpLexerTK_LT:
			depth++
		case csharpparser.CSharpLexerTK_GT:
			depth--
			if depth == 0 {
				return cursor + 1, true
			}
		case csharpparser.CSharpLexerTK_LBRACE,
			csharpparser.CSharpLexerTK_RBRACE,
			csharpparser.CSharpLexerTK_SEMI:
			return index, false
		}
	}
	return index, false
}

// csharpNamespaceOrTypeName validates the small grammar shared by namespace
// declarations and using targets, including contextual identifiers, qualified
// aliases and generic type arguments. Returning the normalized token text lets
// dependency matching retain the longest namespace prefix.
func csharpNamespaceOrTypeName(tokens []csharpScanToken) (string, bool) {
	if len(tokens) == 0 || !isCSharpIdentifierToken(tokens[0]) {
		return "", false
	}
	index := 1
	if index < len(tokens) && tokens[index].typ == csharpparser.CSharpLexerTK_COLON_COLON {
		index++
		if index >= len(tokens) || !isCSharpIdentifierToken(tokens[index]) {
			return "", false
		}
		index++
	}
	var ok bool
	if index, ok = consumeCSharpTypeArguments(tokens, index); !ok {
		return "", false
	}
	for index < len(tokens) {
		if tokens[index].typ != csharpparser.CSharpLexerTK_DOT ||
			index+1 >= len(tokens) || !isCSharpIdentifierToken(tokens[index+1]) {
			return "", false
		}
		index += 2
		if index, ok = consumeCSharpTypeArguments(tokens, index); !ok {
			return "", false
		}
	}
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		parts = append(parts, token.text)
	}
	return strings.Join(parts, ""), true
}

func scanCSharpUsing(tokens []csharpScanToken, start int) (csharpScannedUsing, string, int, bool) {
	if start >= len(tokens) {
		return csharpScannedUsing{}, "", start, false
	}
	index := start
	isGlobal := tokens[index].typ == csharpparser.CSharpLexerKW_GLOBAL
	if isGlobal {
		index++
	}
	if index >= len(tokens) || tokens[index].typ != csharpparser.CSharpLexerKW_USING {
		return csharpScannedUsing{}, "", start, false
	}
	usingIndex := index
	end := usingIndex + 1
	for ; end < len(tokens); end++ {
		switch tokens[end].typ {
		case csharpparser.CSharpLexerTK_SEMI:
			goto foundSemicolon
		case csharpparser.CSharpLexerTK_LBRACE, csharpparser.CSharpLexerTK_RBRACE:
			return csharpScannedUsing{}, "", start, false
		}
	}
	return csharpScannedUsing{}, "", start, false

foundSemicolon:
	valueStart := usingIndex + 1
	if valueStart < end && tokens[valueStart].typ == csharpparser.CSharpLexerKW_STATIC {
		valueStart++
	}
	if valueStart >= end {
		return csharpScannedUsing{}, "", start, false
	}
	// An alias assignment is the only '=' accepted by a using directive and
	// must immediately follow its single identifier. This rejects top-level
	// using declarations such as `using var handle = Open();`.
	if valueStart+1 < end && isCSharpIdentifierToken(tokens[valueStart]) &&
		tokens[valueStart+1].typ == csharpparser.CSharpLexerTK_EQ {
		valueStart += 2
	} else {
		for cursor := valueStart; cursor < end; cursor++ {
			if tokens[cursor].typ == csharpparser.CSharpLexerTK_EQ {
				return csharpScannedUsing{}, "", start, false
			}
		}
	}
	target, ok := csharpNamespaceOrTypeName(tokens[valueStart:end])
	if !ok {
		return csharpScannedUsing{}, "", start, false
	}
	parts := make([]string, 0, end-start+1)
	for _, token := range tokens[start : end+1] {
		parts = append(parts, token.text)
	}
	return csharpScannedUsing{target: target, global: isGlobal}, strings.Join(parts, " "), end, true
}

func namespaceOnlyScope(scopes []bool) bool {
	for _, namespace := range scopes {
		if !namespace {
			return false
		}
	}
	return true
}

func activeCSharpNamespace(scopeNamespaces []string, fileScopedNamespace string) string {
	for index := len(scopeNamespaces) - 1; index >= 0; index-- {
		if scopeNamespaces[index] != "" {
			return scopeNamespaces[index]
		}
	}
	return fileScopedNamespace
}

// scanCSharpFile uses one lexer pass for all project-planning facts. Hidden
// channel trivia, comments, strings and inactive preprocessor sections cannot
// become namespace/import candidates. Namespace braces are tracked separately
// so directives in namespace bodies are accepted while using statements in
// types and methods are ignored.
func scanCSharpFile(source string) csharpFileScan {
	tokens := defaultCSharpTokens(source)
	scan := csharpFileScan{}
	namespaceBraces := make(map[int]string)
	scopes := make([]bool, 0)
	scopeNamespaces := make([]string, 0)
	fileScopedNamespace := ""

	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		switch token.typ {
		case csharpparser.CSharpLexerTK_LBRACE:
			namespaceName, namespace := namespaceBraces[index]
			scopes = append(scopes, namespace)
			scopeNamespaces = append(scopeNamespaces, namespaceName)
			continue
		case csharpparser.CSharpLexerTK_RBRACE:
			if len(scopes) > 0 {
				scopes = scopes[:len(scopes)-1]
				scopeNamespaces = scopeNamespaces[:len(scopeNamespaces)-1]
			}
			continue
		}
		if !namespaceOnlyScope(scopes) {
			continue
		}

		if token.typ == csharpparser.CSharpLexerKW_NAMESPACE {
			end := index + 1
			for end < len(tokens) && tokens[end].typ != csharpparser.CSharpLexerTK_LBRACE &&
				tokens[end].typ != csharpparser.CSharpLexerTK_SEMI {
				if tokens[end].typ == csharpparser.CSharpLexerTK_RBRACE {
					break
				}
				end++
			}
			if end < len(tokens) && end > index+1 {
				if declaredName, ok := csharpNamespaceOrTypeName(tokens[index+1 : end]); ok {
					namespaceName := declaredName
					if parent := activeCSharpNamespace(scopeNamespaces, fileScopedNamespace); parent != "" {
						namespaceName = parent + "." + declaredName
					}
					if scan.namespaceName == "" {
						scan.namespaceName = namespaceName
					}
					scan.namespaceNames = appendUnique(scan.namespaceNames, namespaceName)
					if tokens[end].typ == csharpparser.CSharpLexerTK_LBRACE {
						namespaceBraces[end] = namespaceName
					} else if len(scopes) == 0 {
						fileScopedNamespace = namespaceName
					}
				}
			}
			continue
		}

		if token.typ != csharpparser.CSharpLexerKW_USING && token.typ != csharpparser.CSharpLexerKW_GLOBAL {
			continue
		}
		using, directive, end, ok := scanCSharpUsing(tokens, index)
		if !ok {
			continue
		}
		index = end
		if using.global && (len(scopes) != 0 || fileScopedNamespace != "") {
			continue
		}
		scan.usings = append(scan.usings, using)
		if using.global {
			scan.globalDirectives = append(scan.globalDirectives, directive)
		}
	}
	return scan
}

// scanCSharpGlobalUsingDirectives remains as the focused helper used by the
// registry tests. Project planning calls scanCSharpFile directly and caches the
// complete result.
func scanCSharpGlobalUsingDirectives(source string) []string {
	return scanCSharpFile(source).globalDirectives
}

// prepareGlobalUsings starts a fresh project lifecycle. Parsing the small
// directive-only source gives aliases a detached structured AST (including
// nested generic arguments), while avoiding a second parse of full source
// files. The detached nodes remain valid after the temporary AST root and the
// real source-file AST roots are released.
func (s *SSABuilder) prepareGlobalUsings(directives []string) {
	snapshot := newGlobalUsingSnapshot()
	if len(directives) == 0 {
		s.globalUsings.replace(snapshot)
		return
	}

	ast, err := Frontend(strings.Join(directives, "\n"))
	if err == nil && ast != nil {
		for _, raw := range ast.AllUsing_directive() {
			directive, _ := raw.(*csharpparser.Using_directiveContext)
			if directive == nil || directive.KW_GLOBAL() == nil {
				continue
			}
			switch {
			case directive.Using_namespace_directive() != nil:
				ns, _ := directive.Using_namespace_directive().(*csharpparser.Using_namespace_directiveContext)
				if ns != nil && ns.Namespace_name() != nil {
					snapshot.addNamespace(ns.Namespace_name().GetText())
				}
			case directive.Using_static_directive() != nil:
				static, _ := directive.Using_static_directive().(*csharpparser.Using_static_directiveContext)
				if static != nil && static.Type_name() != nil {
					snapshot.addStatic(static.Type_name().GetText())
				}
			case directive.Using_alias_directive() != nil:
				alias, _ := directive.Using_alias_directive().(*csharpparser.Using_alias_directiveContext)
				if alias != nil && alias.Namespace_or_type_name() != nil {
					node := ssa.DetachAST(alias.Namespace_or_type_name())
					snapshot.addAlias(identText(alias.Identifier()), alias.Namespace_or_type_name().GetText(), node)
				}
			}
		}
		ssa.ReleaseASTRoot(ast)
	}
	s.globalUsings.replace(snapshot)
}
