package ts2ssa

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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

const EXPORT_EQUAL_VALUE = "EXPORT_EQUAL_VALUE"

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

	fs := b.GetProgram().Loader.GetFilesysFileSystem()

	// 如果导入路径已经有扩展名，直接使用
	if hasValidExtension(importPath) {
		if !fs.IsAbs(importPath) {
			candidates = append(candidates, fs.Join(baseDir, importPath))
		} else {

		}

	} else {
		// 尝试添加各种扩展名
		if !fs.IsAbs(importPath) {
			for _, ext := range extensions {
				candidates = append(candidates, fs.Join(baseDir, importPath+ext))
			}

			// 尝试index文件
			for _, ext := range extensions {
				candidates = append(candidates, fs.Join(baseDir, importPath, "index"+ext))
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

// getEntityNameText 获取实体名称的文本
// 用于处理 EntityName（Identifier 或 QualifiedName，如 A.B.C）
func (b *builder) getEntityNameText(entityName *ast.ModuleReference) string {
	if entityName == nil {
		return ""
	}

	switch entityName.Kind {
	case ast.KindIdentifier:
		// 简单标识符: import A = B
		return entityName.AsIdentifier().Text

	case ast.KindQualifiedName:
		// 限定名: import A = B.C.D
		// QualifiedName 是递归结构: Left.Right
		return b.getQualifiedNameText(entityName.AsQualifiedName())

	default:
		return ""
	}
}

// getQualifiedNameText 递归获取限定名的完整文本
// 例如: A.B.C 会被递归处理为 "A.B.C"
func (b *builder) getQualifiedNameText(qualifiedName *ast.QualifiedName) string {
	if qualifiedName == nil {
		return ""
	}

	// QualifiedName 结构: Left.Right
	// Left 可以是 Identifier 或另一个 QualifiedName
	// Right 总是 Identifier

	var leftText string
	if qualifiedName.Left != nil {
		switch qualifiedName.Left.Kind {
		case ast.KindIdentifier:
			leftText = qualifiedName.Left.AsIdentifier().Text
		case ast.KindQualifiedName:
			leftText = b.getQualifiedNameText(qualifiedName.Left.AsQualifiedName())
		}
	}

	var rightText string
	if qualifiedName.Right != nil {
		rightText = qualifiedName.Right.AsIdentifier().Text
	}

	if leftText != "" && rightText != "" {
		return leftText + "." + rightText
	} else if leftText != "" {
		return leftText
	} else {
		return rightText
	}
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

// ImportExternLibValue 分配导入的值
func (b *builder) ImportExternLibValue(localName, exportName, modulePath string, isExternal bool) {
	if localName == "" {
		return
	}

	var value ssa.Value

	if isExternal {
		return
	} else {
		// 本地模块：尝试获取实际值
		value = b.getModuleExportValue(modulePath, exportName)
	}
	_ = value
}

func (b *builder) addImport(resolvedPath, originalName, localName string) {
	if b.importTbl == nil {
		b.importTbl = make(map[string]map[string]string)
	}

	if _, ok := b.importTbl[resolvedPath]; !ok {
		b.importTbl[resolvedPath] = make(map[string]string)
	}

	b.importTbl[resolvedPath][originalName] = localName
}

// bindNamespaceImport 绑定命名空间导入
func (b *builder) bindNamespaceImport(localName, modulePath string, isExternal bool) {
	if localName == "" || !b.PreHandler() {
		return
	}

	var value ssa.Value

	if isExternal {
		return
	} else {
		// 本地模块：创建包含所有导出的容器
		value = b.createLocalNamespaceObject(modulePath, localName)
	}
	_ = value

	log.Infof("Namespace import binding: %s -> * from %s (external: %t)", localName, modulePath, isExternal)
}

// createLocalNamespaceObject 为本地模块创建命名空间对象
func (b *builder) createLocalNamespaceObject(modulePath string, localName string) ssa.Value {
	prog := b.GetProgram()
	if prog == nil {
		return nil
	}

	// 创建命名空间容器
	namespaceObj := b.CreateBlueprint(localName)
	namespaceObj.AddLazyBuilder(func() {
		// 尝试获取模块Program
		moduleProgram, exists := prog.GetLibrary(modulePath)
		if !exists {
			return
		}
		cnt := 0
		// 从模块的ExportValue中添加所有导出
		if moduleProgram.ExportValue != nil {
			for exportName, exportValue := range moduleProgram.ExportValue {
				namespaceObj.RegisterStaticMember(exportName, exportValue)
				cnt++
			}
		}
		container := moduleProgram.GlobalVariablesBlueprint.Container()
		if container != nil {
			for i, m := range container.GetAllMember() {
				namespaceObj.RegisterStaticMember(i.String(), m)
				cnt++
			}
		}
		log.Infof("Created local namespace object for module: %s with %d exports", modulePath, cnt)
	})
	namespaceObj.Build()

	return namespaceObj.Container()
}

// resolveExportValueAndType 递归解析导出值和类型，处理重导出链
// 返回值: (value ssa.Value, type ssa.Type)
func (b *builder) resolveExportValueAndType(prog *ssa.Program, lib *ssa.Program, exportName string) (ssa.Value, ssa.Type) {
	return b.resolveExportValueAndTypeRecursive(prog, lib, exportName, make(map[string]bool))
}

// resolveExportValueAndTypeRecursive 递归解析导出值和类型的内部实现
// visited 用于防止循环引用
func (b *builder) resolveExportValueAndTypeRecursive(prog *ssa.Program, lib *ssa.Program, exportName string, visited map[string]bool) (ssa.Value, ssa.Type) {
	if lib == nil {
		return nil, nil
	}

	// 生成唯一的库+导出名标识，用于防止循环引用
	visitKey := lib.GetProgramName() + ":" + exportName
	if visited[visitKey] {
		return nil, nil
	}
	visited[visitKey] = true

	// 首先尝试直接获取导出值和类型
	externValue := lib.GetExportValue(exportName)
	externalType, typeOk := lib.GetExportType(exportName)

	// 如果找到了值或类型，直接返回
	if !utils.IsNil(externValue) || typeOk {
		return externValue, externalType
	}

	// 如果没有找到，检查是否有重导出信息
	info := lib.GetReExportInfo(exportName)
	wildCardInfo := lib.GetWildCardReExportInfo()
	if info == nil {
		if wildCardInfo != nil {
			info = wildCardInfo
		} else {
			return nil, nil
		}
	}

	// 获取重导出的源库
	realLib, found := prog.GetLibrary(info.FilePath)
	if !found {
		return nil, nil
	}

	// 处理命名空间导出 (export * as ns from './mod')
	if info.IsNameSpaceExport {
		variable := b.CreateVariable(exportName)
		value := b.createLocalNamespaceObject(info.FilePath, exportName)
		b.AssignVariable(variable, value)
		return value, nil
	}

	// 处理通配符导出 (export * from './mod')
	if info.IsWildCardExport {
		// 通配符导出，需要递归查找
		// 因为源库也可能使用了重导出（包括通配符重导出）
		return b.resolveExportValueAndTypeRecursive(prog, realLib, exportName, visited)
	}

	// 处理普通重导出，递归查找
	// 如果有指定的导出名，使用指定的名称，否则使用原名称
	targetName := exportName
	if info.ExportName != "" {
		targetName = info.ExportName
	}

	return b.resolveExportValueAndTypeRecursive(prog, realLib, targetName, visited)
}

// IsTopLevelNodeInAST 判断某个节点是否为 AST 的顶层语句
// 顶层语句是直接位于 SourceFile.Statements 中的语句节点
func (b *builder) IsTopLevelNodeInAST(node *ast.Node) bool {
	if node == nil {
		return false
	}

	// 方法1：检查父节点是否为 SourceFile
	// 在某些情况下，语句节点的 Parent 直接指向 SourceFile
	if node.Parent != nil && ast.IsSourceFile(node.Parent) {
		return true
	}

	// 方法2：检查当前 sourceFile 的 Statements 列表中是否包含此节点
	// 这是更可靠的方法，因为有些节点的 Parent 可能不是直接指向 SourceFile
	if b.sourceFile != nil && b.sourceFile.Statements != nil {
		for _, stmt := range b.sourceFile.Statements.Nodes {
			if stmt == node {
				return true
			}
		}
	}

	return false
}
