package javaparser

import (
	"errors"
	"fmt"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

type predictionRuleFixture struct {
	name, source string
	entry        func(*JavaParser) antlr.Tree
}

var predictionRuleFixtures = []predictionRuleFixture{
	{"compilationUnit", "package p; import q.T; class C { T<X> f(T x){return x;} }", func(p *JavaParser) antlr.Tree { return p.CompilationUnit() }},
	{"packageDeclaration", "@A package a.b;", func(p *JavaParser) antlr.Tree { return p.PackageDeclaration() }},
	{"packageName", "${package}.a", func(p *JavaParser) antlr.Tree { return p.PackageName() }},
	{"importDeclaration", "import static a.b.C.*;", func(p *JavaParser) antlr.Tree { return p.ImportDeclaration() }},
	{"typeDeclaration", "sealed class C<T> extends X implements Y permits Z {}", func(p *JavaParser) antlr.Tree { return p.TypeDeclaration() }},
	{"modifiers", "public static final @A", func(p *JavaParser) antlr.Tree { return p.Modifiers() }},
	{"modifier", "@A", func(p *JavaParser) antlr.Tree { return p.Modifier() }},
	{"staticModifier", "synchronized", func(p *JavaParser) antlr.Tree { return p.StaticModifier() }},
	{"classOrInterfaceModifier", "sealed", func(p *JavaParser) antlr.Tree { return p.ClassOrInterfaceModifier() }},
	{"staticClassModifier", "non-sealed", func(p *JavaParser) antlr.Tree { return p.StaticClassModifier() }},
	{"variableModifier", "@A", func(p *JavaParser) antlr.Tree { return p.VariableModifier() }},
	{"classDeclaration", "class C<T> extends X implements Y permits Z {}", func(p *JavaParser) antlr.Tree { return p.ClassDeclaration() }},
	{"typeParameters", "<@A T extends X & Y,U>", func(p *JavaParser) antlr.Tree { return p.TypeParameters() }},
	{"typeParameter", "@A T extends X & Y", func(p *JavaParser) antlr.Tree { return p.TypeParameter() }},
	{"typeBound", "X & Y", func(p *JavaParser) antlr.Tree { return p.TypeBound() }},
	{"enumDeclaration", "enum E implements X { @A A(1){int x;}, B; int y; }", func(p *JavaParser) antlr.Tree { return p.EnumDeclaration() }},
	{"enumConstants", "A(1), B", func(p *JavaParser) antlr.Tree { return p.EnumConstants() }},
	{"enumConstant", "@A A(1){int x;}", func(p *JavaParser) antlr.Tree { return p.EnumConstant() }},
	{"enumBodyDeclarations", "; int x;", func(p *JavaParser) antlr.Tree { return p.EnumBodyDeclarations() }},
	{"interfaceDeclaration", "interface I<T> extends X permits Y {}", func(p *JavaParser) antlr.Tree { return p.InterfaceDeclaration() }},
	{"classBody", "{ T<X> x; C(){} }", func(p *JavaParser) antlr.Tree { return p.ClassBody() }},
	{"interfaceBody", "{ int X=1; default void m(){} }", func(p *JavaParser) antlr.Tree { return p.InterfaceBody() }},
	{"classBodyDeclaration", "public T<X> m(){return x;}", func(p *JavaParser) antlr.Tree { return p.ClassBodyDeclaration() }},
	{"memberDeclaration", "T<X> m(T x){return x;}", func(p *JavaParser) antlr.Tree { return p.MemberDeclaration() }},
	{"methodDeclaration", "@A T<X> m(T... x)[] throws E {}", func(p *JavaParser) antlr.Tree { return p.MethodDeclaration() }},
	{"methodBody", "{}", func(p *JavaParser) antlr.Tree { return p.MethodBody() }},
	{"typeTypeOrVoid", "T<@A X> @B []", func(p *JavaParser) antlr.Tree { return p.TypeTypeOrVoid() }},
	{"genericMethodDeclaration", "<T> T m(T x){return x;}", func(p *JavaParser) antlr.Tree { return p.GenericMethodDeclaration() }},
	{"genericConstructorDeclaration", "<T> C(T x){}", func(p *JavaParser) antlr.Tree { return p.GenericConstructorDeclaration() }},
	{"constructorDeclaration", "C(T x) throws E {}", func(p *JavaParser) antlr.Tree { return p.ConstructorDeclaration() }},
	{"compactConstructorDeclaration", "public C {}", func(p *JavaParser) antlr.Tree { return p.CompactConstructorDeclaration() }},
	{"fieldDeclaration", "T<X> x=f(a->a), y[];", func(p *JavaParser) antlr.Tree { return p.FieldDeclaration() }},
	{"interfaceBodyDeclaration", "default void m(){}", func(p *JavaParser) antlr.Tree { return p.InterfaceBodyDeclaration() }},
	{"interfaceMemberDeclaration", "<T> T m(T x);", func(p *JavaParser) antlr.Tree { return p.InterfaceMemberDeclaration() }},
	{"constDeclaration", "int X=1,Y[]={2};", func(p *JavaParser) antlr.Tree { return p.ConstDeclaration() }},
	{"constantDeclarator", "X[]={1,2}", func(p *JavaParser) antlr.Tree { return p.ConstantDeclarator() }},
	{"interfaceMethodDeclaration", "default T m(){return x;}", func(p *JavaParser) antlr.Tree { return p.InterfaceMethodDeclaration() }},
	{"interfaceMethodModifier", "default", func(p *JavaParser) antlr.Tree { return p.InterfaceMethodModifier() }},
	{"genericInterfaceMethodDeclaration", "public <T> T m(T x);", func(p *JavaParser) antlr.Tree { return p.GenericInterfaceMethodDeclaration() }},
	{"interfaceCommonBodyDeclaration", "@A T m(T x) throws E;", func(p *JavaParser) antlr.Tree { return p.InterfaceCommonBodyDeclaration() }},
	{"variableDeclarators", "x=f(a->a), y[]", func(p *JavaParser) antlr.Tree { return p.VariableDeclarators() }},
	{"variableDeclarator", "x[]=f(a->a)", func(p *JavaParser) antlr.Tree { return p.VariableDeclarator() }},
	{"variableDeclaratorId", "x[][]", func(p *JavaParser) antlr.Tree { return p.VariableDeclaratorId() }},
	{"variableInitializer", "new T[]{x,y}", func(p *JavaParser) antlr.Tree { return p.VariableInitializer() }},
	{"arrayInitializer", "{{x},{y},}", func(p *JavaParser) antlr.Tree { return p.ArrayInitializer() }},
	{"classOrInterfaceType", "a.b.T<X>.U<Y>", func(p *JavaParser) antlr.Tree { return p.ClassOrInterfaceType() }},
	{"typeArgument", "@A ? extends T<X>[]", func(p *JavaParser) antlr.Tree { return p.TypeArgument() }},
	{"qualifiedNameList", "a.E,b.F", func(p *JavaParser) antlr.Tree { return p.QualifiedNameList() }},
	{"formalParameters", "(C this, final T x, @A U... y)", func(p *JavaParser) antlr.Tree { return p.FormalParameters() }},
	{"receiverParameter", "C Outer.this", func(p *JavaParser) antlr.Tree { return p.ReceiverParameter() }},
	{"formalParameterList", "final T x,@A U... y", func(p *JavaParser) antlr.Tree { return p.FormalParameterList() }},
	{"formalParameter", "@A final T<X>[] x[]", func(p *JavaParser) antlr.Tree { return p.FormalParameter() }},
	{"lastFormalParameter", "final T @A ... x", func(p *JavaParser) antlr.Tree { return p.LastFormalParameter() }},
	{"lambdaLVTIList", "final var x,@A var y", func(p *JavaParser) antlr.Tree { return p.LambdaLVTIList() }},
	{"lambdaLVTIParameter", "@A final var x", func(p *JavaParser) antlr.Tree { return p.LambdaLVTIParameter() }},
	{"qualifiedName", "module.record.yield", func(p *JavaParser) antlr.Tree { return p.QualifiedName() }},
	{"literal", "\"\"\"\ntext\n\"\"\"", func(p *JavaParser) antlr.Tree { return p.Literal() }},
	{"integerLiteral", "0xCAFE", func(p *JavaParser) antlr.Tree { return p.IntegerLiteral() }},
	{"floatLiteral", "0x1.fp2", func(p *JavaParser) antlr.Tree { return p.FloatLiteral() }},
	{"altAnnotationQualifiedName", "a.b.@C", func(p *JavaParser) antlr.Tree { return p.AltAnnotationQualifiedName() }},
	{"annotation", "@A(x=1,y={@B,2})", func(p *JavaParser) antlr.Tree { return p.Annotation() }},
	{"elementValuePairs", "x=1,y={@B,2}", func(p *JavaParser) antlr.Tree { return p.ElementValuePairs() }},
	{"elementValuePair", "x={@A,1}", func(p *JavaParser) antlr.Tree { return p.ElementValuePair() }},
	{"elementValue", "@A(x=1)", func(p *JavaParser) antlr.Tree { return p.ElementValue() }},
	{"elementValueArrayInitializer", "{@A,1,}", func(p *JavaParser) antlr.Tree { return p.ElementValueArrayInitializer() }},
	{"annotationTypeDeclaration", "@interface A { int x() default 1; }", func(p *JavaParser) antlr.Tree { return p.AnnotationTypeDeclaration() }},
	{"annotationTypeBody", "{ int x() default 1; }", func(p *JavaParser) antlr.Tree { return p.AnnotationTypeBody() }},
	{"annotationTypeElementDeclaration", "public int x() default 1;", func(p *JavaParser) antlr.Tree { return p.AnnotationTypeElementDeclaration() }},
	{"annotationTypeElementRest", "int x() default 1;", func(p *JavaParser) antlr.Tree { return p.AnnotationTypeElementRest() }},
	{"annotationMethodOrConstantRest", "x() default 1", func(p *JavaParser) antlr.Tree { return p.AnnotationMethodOrConstantRest() }},
	{"annotationMethodRest", "x() default {1,2}", func(p *JavaParser) antlr.Tree { return p.AnnotationMethodRest() }},
	{"annotationConstantRest", "X=1,Y=2", func(p *JavaParser) antlr.Tree { return p.AnnotationConstantRest() }},
	{"defaultValue", "default @A", func(p *JavaParser) antlr.Tree { return p.DefaultValue() }},
	{"moduleDeclaration", "open module a { requires transitive b; exports a.b to c; }", func(p *JavaParser) antlr.Tree { return p.ModuleDeclaration() }},
	{"moduleBody", "{ requires static transitive a; provides a.I with b.C; }", func(p *JavaParser) antlr.Tree { return p.ModuleBody() }},
	{"moduleDirective", "provides a.I with b.C;", func(p *JavaParser) antlr.Tree { return p.ModuleDirective() }},
	{"requiresModifier", "transitive", func(p *JavaParser) antlr.Tree { return p.RequiresModifier() }},
	{"recordDeclaration", "record R<T>(T x) implements I { public R {} }", func(p *JavaParser) antlr.Tree { return p.RecordDeclaration() }},
	{"recordHeader", "(T x,int y)", func(p *JavaParser) antlr.Tree { return p.RecordHeader() }},
	{"recordComponentList", "T x,int y", func(p *JavaParser) antlr.Tree { return p.RecordComponentList() }},
	{"recordComponent", "T<X> x", func(p *JavaParser) antlr.Tree { return p.RecordComponent() }},
	{"recordBody", "{ public R {} T m(){return x;} }", func(p *JavaParser) antlr.Tree { return p.RecordBody() }},
	{"blockOrState", "if(x) f(); else {}", func(p *JavaParser) antlr.Tree { return p.BlockOrState() }},
	{"block", "{ T<X> x; return x; }", func(p *JavaParser) antlr.Tree { return p.Block() }},
	{"elseBlock", "else if(x) f();", func(p *JavaParser) antlr.Tree { return p.ElseBlock() }},
	{"elseIfBlock", "else if(x) f();", func(p *JavaParser) antlr.Tree { return p.ElseIfBlock() }},
	{"blockStatementList", "T<X> x; f(x);", func(p *JavaParser) antlr.Tree { return p.BlockStatementList() }},
	{"blockStatement", "T<X> x=f(a->a);", func(p *JavaParser) antlr.Tree { return p.BlockStatement() }},
	{"localVariableDeclaration", "final T<X>[] x={a,b}", func(p *JavaParser) antlr.Tree { return p.LocalVariableDeclaration() }},
	{"identifier", "yield", func(p *JavaParser) antlr.Tree { return p.Identifier() }},
	{"typeIdentifier", "record", func(p *JavaParser) antlr.Tree { return p.TypeIdentifier() }},
	{"localTypeDeclaration", "final record R(T x){}", func(p *JavaParser) antlr.Tree { return p.LocalTypeDeclaration() }},
	{"statement", "if(x) if(y) f(); else g();", func(p *JavaParser) antlr.Tree { return p.Statement() }},
	{"statementList", "f(); return x;", func(p *JavaParser) antlr.Tree { return p.StatementList() }},
	{"switchStatement", "switch(x){case 1: f(); break; default: g();}", func(p *JavaParser) antlr.Tree { return p.SwitchStatement() }},
	{"switchBlockStatementGroup", "case 1,2: f(); break;", func(p *JavaParser) antlr.Tree { return p.SwitchBlockStatementGroup() }},
	{"switchLabel", "case x,y:", func(p *JavaParser) antlr.Tree { return p.SwitchLabel() }},
	{"ifstmt", "if(x) {} else if(y) f(); else {}", func(p *JavaParser) antlr.Tree { return p.Ifstmt() }},
	{"catchClause", "catch(final E|F e){}", func(p *JavaParser) antlr.Tree { return p.CatchClause() }},
	{"catchType", "a.E|b.F", func(p *JavaParser) antlr.Tree { return p.CatchType() }},
	{"finallyBlock", "finally {}", func(p *JavaParser) antlr.Tree { return p.FinallyBlock() }},
	{"resourceSpecification", "(final T x=f();var y=g();)", func(p *JavaParser) antlr.Tree { return p.ResourceSpecification() }},
	{"resources", "T x=f();var y=g()", func(p *JavaParser) antlr.Tree { return p.Resources() }},
	{"resource", "var x=f()", func(p *JavaParser) antlr.Tree { return p.Resource() }},
	{"forControl", "T<X> x:xs", func(p *JavaParser) antlr.Tree { return p.ForControl() }},
	{"forInit", "int x=0,y=1", func(p *JavaParser) antlr.Tree { return p.ForInit() }},
	{"enhancedForControl", "final T<X>[] x:xs", func(p *JavaParser) antlr.Tree { return p.EnhancedForControl() }},
	{"parExpression", "(a?b:c)", func(p *JavaParser) antlr.Tree { return p.ParExpression() }},
	{"parExpressionList", "(a,b,)", func(p *JavaParser) antlr.Tree { return p.ParExpressionList() }},
	{"expressionList", "a,f(x->x),", func(p *JavaParser) antlr.Tree { return p.ExpressionList() }},
	{"methodCall", "f(x->x)", func(p *JavaParser) antlr.Tree { return p.MethodCall() }},
	{"expression", "(T & U) x", func(p *JavaParser) antlr.Tree { return p.Expression() }},
	{"leftMemberCall", ".yield", func(p *JavaParser) antlr.Tree { return p.LeftMemberCall() }},
	{"leftSliceCall", "[f(x->x)]", func(p *JavaParser) antlr.Tree { return p.LeftSliceCall() }},
	{"pattern", "final T<X> @A x", func(p *JavaParser) antlr.Tree { return p.Pattern() }},
	{"lambdaExpression", "(x,y)->x+y", func(p *JavaParser) antlr.Tree { return p.LambdaExpression() }},
	{"lambdaParameters", "(@A final var x,var y)", func(p *JavaParser) antlr.Tree { return p.LambdaParameters() }},
	{"lambdaBody", "{T<X> x; return x;}", func(p *JavaParser) antlr.Tree { return p.LambdaBody() }},
	{"primary", "T<X>[].class", func(p *JavaParser) antlr.Tree { return p.Primary() }},
	{"switchExpression", "switch(x){case 1 -> 2; default -> 3;}", func(p *JavaParser) antlr.Tree { return p.SwitchExpression() }},
	{"switchLabeledRule", "case T t && p(t) -> {yield t;}", func(p *JavaParser) antlr.Tree { return p.SwitchLabeledRule() }},
	{"defaultLabeledRule", "case null, default -> 1;", func(p *JavaParser) antlr.Tree { return p.DefaultLabeledRule() }},
	{"guardedPattern", "(T t && p(t)) && q(t)", func(p *JavaParser) antlr.Tree { return p.GuardedPattern() }},
	{"switchRuleOutcome", "return 1;", func(p *JavaParser) antlr.Tree { return p.SwitchRuleOutcome() }},
	{"classType", "a.T<X>.@A U<Y>", func(p *JavaParser) antlr.Tree { return p.ClassType() }},
	{"creator", "<X> T<X>(x){int y;}", func(p *JavaParser) antlr.Tree { return p.Creator() }},
	{"createdName", "a.T<X>.U<>", func(p *JavaParser) antlr.Tree { return p.CreatedName() }},
	{"innerCreator", "U<X>(x){}", func(p *JavaParser) antlr.Tree { return p.InnerCreator() }},
	{"arrayCreatorRest", "[x][y][]", func(p *JavaParser) antlr.Tree { return p.ArrayCreatorRest() }},
	{"classCreatorRest", "(x->x){}", func(p *JavaParser) antlr.Tree { return p.ClassCreatorRest() }},
	{"explicitGenericInvocation", "<T<X>>f(x)", func(p *JavaParser) antlr.Tree { return p.ExplicitGenericInvocation() }},
	{"typeArgumentsOrDiamond", "<>", func(p *JavaParser) antlr.Tree { return p.TypeArgumentsOrDiamond() }},
	{"nonWildcardTypeArgumentsOrDiamond", "<T<X>>", func(p *JavaParser) antlr.Tree { return p.NonWildcardTypeArgumentsOrDiamond() }},
	{"nonWildcardTypeArguments", "<T<X>,U[]>", func(p *JavaParser) antlr.Tree { return p.NonWildcardTypeArguments() }},
	{"typeList", "T<X>,U[]", func(p *JavaParser) antlr.Tree { return p.TypeList() }},
	{"typeType", "@A a.T<X> @B []", func(p *JavaParser) antlr.Tree { return p.TypeType() }},
	{"primitiveType", "int", func(p *JavaParser) antlr.Tree { return p.PrimitiveType() }},
	{"typeArguments", "<@A ? extends T<X>[],U>", func(p *JavaParser) antlr.Tree { return p.TypeArguments() }},
	{"superSuffix", ".<X>f(x)", func(p *JavaParser) antlr.Tree { return p.SuperSuffix() }},
	{"arguments", "(x,f(y->y),)", func(p *JavaParser) antlr.Tree { return p.Arguments() }},
}

type rulePredictionResult struct {
	snapshot    string
	err         error
	next, index int
}

func parsePredictionRule(source string, fast bool, entry func(*JavaParser) antlr.Tree, reference *predictionAutomata) rulePredictionResult {
	lexical := antlr4util.NewErrorListener()
	var next, index int
	if reference == nil {
		tree, err := antlr4util.ParseASTWithSLLFirst(source, func(input antlr.CharStream) *predictionLexer { return &predictionLexer{NewJavaLexer(input), lexical} }, func(input antlr.TokenStream) *JavaParser {
			p := NewJavaParser(input)
			p.SetFastPrediction(fast)
			return p
		}, nil, nil, func(p *JavaParser) antlr.Tree {
			tree := entry(p)
			next, index = p.GetTokenStream().LA(1), p.GetTokenStream().Index()
			return tree
		})
		return rulePredictionResult{predictionSnapshot(tree), errors.Join(err, lexical.Error()), next, index}
	}
	lexer := NewJavaLexer(antlr.NewInputStream(source))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(lexical)
	p := NewJavaParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	reference.apply(p)
	p.SetFastPrediction(fast)
	p.GetInterpreter().SetPredictionMode(antlr.PredictionModeLL)
	p.RemoveErrorListeners()
	p.AddErrorListener(lexical)
	tree := entry(p)
	return rulePredictionResult{predictionSnapshot(tree), lexical.Error(), p.GetTokenStream().LA(1), p.GetTokenStream().Index()}
}

func checkPredictionRule(t *testing.T, source string, entry func(*JavaParser) antlr.Tree, reference *predictionAutomata, valid bool) {
	t.Helper()
	want := parsePredictionRule(source, false, entry, nil)
	got := parsePredictionRule(source, true, entry, nil)
	if fmt.Sprint(want.err) != fmt.Sprint(got.err) || want.next != got.next || want.index != got.index || want.snapshot != got.snapshot {
		t.Fatalf("rule prediction changed for %q: original=%v/%d/%d fast=%v/%d/%d\noriginal: %.1400s\nfast: %.1400s", source, want.err, want.next, want.index, got.err, got.next, got.index, want.snapshot, got.snapshot)
	}
	if valid && (want.err != nil || want.next != antlr.TokenEOF) {
		t.Fatalf("invalid rule fixture %q: %v, next %d", source, want.err, want.next)
	}
	// Public entries accept prefixes; SLL and LL can choose different remaining
	// suffixes. The independent LL oracle applies to fully consumed valid rules.
	if reference != nil && want.err == nil && want.next == antlr.TokenEOF {
		ll := parsePredictionRule(source, false, entry, reference)
		if ll.err != nil || ll.next != want.next || ll.index != want.index || ll.snapshot != want.snapshot {
			t.Fatalf("independent LL mismatch for %q: %v, next %d", source, ll.err, ll.next)
		}
	}
}

func TestPredictionEveryRuleEntry(t *testing.T) {
	rules := NewJavaParser(nil).GetRuleNames()
	if len(rules) != len(predictionRuleFixtures) {
		t.Fatalf("rule fixtures %d != grammar rules %d", len(predictionRuleFixtures), len(rules))
	}
	reference := newPredictionAutomata()
	for i, fixture := range predictionRuleFixtures {
		if fixture.name != rules[i] {
			t.Fatalf("fixture %s does not match rule %s", fixture.name, rules[i])
		}
		t.Run(fixture.name, func(t *testing.T) { checkPredictionRule(t, fixture.source, fixture.entry, reference, true) })
	}
}

func TestPredictionRuleEntryMutations(t *testing.T) {
	reference := newPredictionAutomata()
	for i, fixture := range predictionRuleFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			for action := 0; action < 8; action++ {
				for _, position := range []int{0, i + 1, i + 7} {
					checkPredictionRule(t, predictionMutation(fixture.source, position, action), fixture.entry, reference, false)
				}
			}
		})
	}
}
