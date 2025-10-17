package ts2ssa

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssalog"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/ast"
)

var arithmeticBinOpTbl = map[ast.Kind]ssa.BinaryOpcode{
	// 普通算术操作
	ast.KindPlusToken:             ssa.OpAdd,
	ast.KindMinusToken:            ssa.OpSub,
	ast.KindAsteriskToken:         ssa.OpMul,
	ast.KindSlashToken:            ssa.OpDiv,
	ast.KindPercentToken:          ssa.OpMod,
	ast.KindAsteriskAsteriskToken: ssa.OpPow,

	// 算术赋值操作
	ast.KindPlusEqualsToken:             ssa.OpAdd,
	ast.KindMinusEqualsToken:            ssa.OpSub,
	ast.KindAsteriskEqualsToken:         ssa.OpMul,
	ast.KindSlashEqualsToken:            ssa.OpDiv,
	ast.KindPercentEqualsToken:          ssa.OpMod,
	ast.KindAsteriskAsteriskEqualsToken: ssa.OpPow,
}

var bitwiseBinOpTbl = map[ast.Kind]ssa.BinaryOpcode{
	// 普通按位操作
	ast.KindAmpersandToken:                         ssa.OpAnd,
	ast.KindBarToken:                               ssa.OpOr,
	ast.KindCaretToken:                             ssa.OpXor,
	ast.KindLessThanLessThanToken:                  ssa.OpShl,
	ast.KindGreaterThanGreaterThanToken:            ssa.OpShr,
	ast.KindGreaterThanGreaterThanGreaterThanToken: ssa.OpShr,

	// 按位赋值操作
	ast.KindAmpersandEqualsToken:                         ssa.OpAnd,
	ast.KindBarEqualsToken:                               ssa.OpOr,
	ast.KindCaretEqualsToken:                             ssa.OpXor,
	ast.KindLessThanLessThanEqualsToken:                  ssa.OpShl,
	ast.KindGreaterThanGreaterThanEqualsToken:            ssa.OpShr,
	ast.KindGreaterThanGreaterThanGreaterThanEqualsToken: ssa.OpShr,
}

var comparisonBinOpTbl = map[ast.Kind]ssa.BinaryOpcode{
	ast.KindLessThanToken:                ssa.OpLt,
	ast.KindGreaterThanToken:             ssa.OpGt,
	ast.KindLessThanEqualsToken:          ssa.OpLtEq,
	ast.KindGreaterThanEqualsToken:       ssa.OpGtEq,
	ast.KindEqualsEqualsToken:            ssa.OpEq,
	ast.KindEqualsEqualsEqualsToken:      ssa.OpEq,
	ast.KindExclamationEqualsToken:       ssa.OpNotEq,
	ast.KindExclamationEqualsEqualsToken: ssa.OpNotEq,
}

// VisitLeftValueExpression 只接收左值
func (b *builder) VisitLeftValueExpression(node *ast.Expression) *ssa.Variable {
	if node == nil || b.IsStop() {
		return nil
	}

	lval, _ := b.VisitExpression(node, true)
	return lval
}

// VisitRightValueExpression 只接收右值
func (b *builder) VisitRightValueExpression(node *ast.Expression) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}
	_, rval := b.VisitExpression(node, false)
	return rval
}

func (b *builder) IsMapLike(val ssa.Value) bool {
	valType := val.GetType()
	if valType != nil {
		return valType.GetTypeKind() == ssa.MapTypeKind || valType.GetTypeKind() == ssa.ObjectTypeKind
	}
	return false

}

func (b *builder) IsListLike(val ssa.Value) bool {
	valType := val.GetType()
	if valType != nil {
		return valType.GetTypeKind() == ssa.SliceTypeKind
	}
	return false

}

func (b *builder) IsObjectLike(val ssa.Value) bool {
	return val.IsObject()
}

func (b *builder) SwitchFunctionBuilder(s *ssa.StoredFunctionBuilder) func() {
	t := b.StoreFunctionBuilder()
	b.LoadBuilder(s)
	return func() {
		b.LoadBuilder(t)
	}
}

func (b *builder) LoadBuilder(s *ssa.StoredFunctionBuilder) {
	b.FunctionBuilder = s.Current
	b.LoadFunctionBuilder(s.Store)
}

func (b *builder) CreateJSVariable(identifierName string) *ssa.Variable {
	if b.useStrict {
		return b.CreateLocalVariable(identifierName)
	}
	return b.CreateVariable(identifierName)
}

// resolveImportLibPath 解析导入路径
func (b *builder) resolveImportLibPath(importPath string) (resolvedPath string, isExternal bool) {
	if importPath == "" {
		return "", false
	}

	// 检查是否是外部模块（不以.或/开头）
	if !strings.HasPrefix(importPath, ".") && !strings.HasPrefix(importPath, "/") {
		return importPath, true // 外部模块
	}

	// 处理相对路径导入
	prog := b.GetProgram()
	if prog == nil || prog.Loader == nil {
		return importPath, false
	}

	editor := b.GetEditor()
	if editor == nil {
		return importPath, false
	}

	dir := editor.GetFolderPath()
	candidates := b.getImportCandidates(importPath, dir)
	for _, candidate := range candidates {
		if exist, err := prog.Loader.GetFilesysFileSystem().Exists(candidate); exist && err == nil {
			return candidate, false
		}
	}
	ssalog.Log.Warnf("Can't find import path for %s", importPath)
	return "", false
}

// getImportCandidates 获取导入路径的候选文件
func (b *builder) getImportCandidates(importPath, baseDir string) []string {
	candidates := []string{}
	extensions := []string{".ts", ".tsx", ".js", ".jsx", ".d.ts"}

	// 如果导入路径已经有扩展名，直接使用
	if hasValidExtension(importPath) {
		if !filepath.IsAbs(importPath) {
			candidates = append(candidates, filepath.Join(baseDir, importPath))
		} else {

		}

	} else {
		// 尝试添加各种扩展名
		if !filepath.IsAbs(importPath) {
			for _, ext := range extensions {
				candidates = append(candidates, filepath.Join(baseDir, importPath+ext))
			}

			// 尝试index文件
			for _, ext := range extensions {
				candidates = append(candidates, filepath.Join(baseDir, importPath, "index"+ext))
			}
		} else {

		}

	}

	return candidates
}

// hasValidExtension 检查文件是否有有效的扩展名
func hasValidExtension(path string) bool {
	// TS support direct json file import but we will not handle json import for now
	validExts := []string{".ts", ".tsx", ".js", ".jsx", ".d.ts", ".mjs", ".cjs"}
	for _, ext := range validExts {
		if strings.HasSuffix(strings.ToLower(path), ext) {
			return true
		}
	}
	return false
}

// createExternalImportPlaceholder 为外部导入创建占位符
func (b *builder) createExternalImportPlaceholder(importPath, exportName string) ssa.Value {
	placeholderName := fmt.Sprintf("external_%s_from_%s", exportName, importPath)
	placeholder := b.EmitConstInstPlaceholder(placeholderName)
	placeholder.SetName(fmt.Sprintf("%s (from %s)", exportName, importPath))
	return placeholder
}

// createNamespaceImportPlaceholder 为命名空间导入创建占位符
func (b *builder) createNamespaceImportPlaceholder(importPath string) ssa.Value {
	placeholderName := fmt.Sprintf("namespace_%s", importPath)
	placeholder := b.EmitConstInstPlaceholder(placeholderName)
	placeholder.SetName(fmt.Sprintf("* as namespace (from %s)", importPath))
	return placeholder
}

// getModuleExportValue 从模块中获取导出值
func (b *builder) getModuleExportValue(modulePath, exportName string) ssa.Value {
	prog := b.GetProgram()
	if prog == nil {
		return b.EmitUndefined(fmt.Sprintf("module_%s_not_found", modulePath))
	}

	// 尝试获取模块Program
	moduleProgram, _ := prog.GetOrCreateLibrary(modulePath)
	if moduleProgram == nil {
		return nil
	}
	moduleProgram.PushEditor(b.GetEditor())

	err := prog.ImportTypeFromLib(moduleProgram, exportName, nil)
	_ = err
	// 从模块的ExportValue中获取导出值
	if exportValue := moduleProgram.GetExportValue(exportName); exportValue != nil {
		return exportValue
	}
	moduleProgram.PopEditor(false)
	return nil
}

// extractModuleSpecifierText 提取模块说明符的文本
func (b *builder) extractModuleSpecifierText(expr *ast.Expression) string {
	if expr == nil {
		return ""
	}

	switch expr.Kind {
	case ast.KindStringLiteral:
		return strings.Trim(expr.AsStringLiteral().Text, `"'`)
	case ast.KindNoSubstitutionTemplateLiteral:
		return strings.Trim(expr.AsNoSubstitutionTemplateLiteral().Text, "`")
	default:
		return ""
	}
}

// getDeclarationNameText 获取声明名称的文本
func (b *builder) getDeclarationNameText(name *ast.DeclarationName) string {
	if name == nil {
		return ""
	}
	if id := name.AsIdentifier(); id != nil {
		return id.Text
	}
	if str := name.AsStringLiteral(); str != nil {
		return strings.Trim(str.Text, `"'`)
	}
	return ""
}

// createExternalModuleLibrary 为外部模块创建占位符Library
func (b *builder) createExternalModuleLibrary(modulePath string) {
	prog := b.GetProgram()
	if prog == nil {
		return
	}

	// 创建或获取外部模块的Library
	if _, err := prog.GetOrCreateLibrary(modulePath); err != nil {
		log.Warnf("Failed to create external library %s: %v", modulePath, err)
	}
}

// assignImportedValue 分配导入的值
func (b *builder) assignImportedValue(localName, exportName, modulePath string, isExternal bool) {
	if localName == "" {
		return
	}

	var value ssa.Value

	if isExternal {
		// 外部模块：创建占位符
		value = b.createExternalImportPlaceholder(modulePath, exportName)
	} else {
		// 本地模块：尝试获取实际值
		value = b.getModuleExportValue(modulePath, exportName)
	}
	_ = value

	// 创建变量并赋值
	if value != nil && !b.PreHandler() {
		variable := b.CreateJSVariable(localName)
		b.AssignVariable(variable, value)
		log.Infof("Import binding: %s -> %s from %s (external: %t)", localName, exportName, modulePath, isExternal)
	}
}

// bindNamespaceImport 绑定命名空间导入
func (b *builder) bindNamespaceImport(localName, modulePath string, isExternal bool) {
	if localName == "" {
		return
	}

	var value ssa.Value

	if isExternal {
		// 外部模块：创建命名空间占位符
		value = b.createNamespaceImportPlaceholder(modulePath)
	} else {
		// 本地模块：创建包含所有导出的容器
		value = b.createLocalNamespaceObject(modulePath)
	}

	// 创建变量并赋值
	variable := b.CreateJSVariable(localName)
	b.AssignVariable(variable, value)

	log.Infof("Namespace import binding: %s -> * from %s (external: %t)", localName, modulePath, isExternal)
}

// createLocalNamespaceObject 为本地模块创建命名空间对象
func (b *builder) createLocalNamespaceObject(modulePath string) ssa.Value {
	prog := b.GetProgram()
	if prog == nil {
		return b.EmitUndefined("namespace_prog_not_found")
	}

	// 尝试获取模块Program
	moduleProgram, exists := prog.GetLibrary(modulePath)
	if !exists {
		return b.EmitUndefined(fmt.Sprintf("namespace_module_%s_not_found", modulePath))
	}

	// 创建命名空间容器
	namespaceObj := b.EmitEmptyContainer()

	// 从模块的ExportValue中添加所有导出
	if moduleProgram.ExportValue != nil {
		for exportName, exportValue := range moduleProgram.ExportValue {
			member := b.CreateMemberCallVariable(namespaceObj, b.EmitConstInst(exportName))
			b.AssignVariable(member, exportValue)
		}
		log.Infof("Created local namespace object for module: %s with %d exports", modulePath, len(moduleProgram.ExportValue))
	}

	return namespaceObj
}

func ShouldVisit(isPreHandle, isExport bool) bool {
	return isExport == isPreHandle
}
