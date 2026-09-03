// Code generated from ./CSharpParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package csharpparser // CSharpParser
import "github.com/yaklang/antlr/v4"

type BaseCSharpParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseCSharpParserVisitor) VisitProg(ctx *ProgContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInput(ctx *InputContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInput_section(ctx *Input_sectionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInput_section_part(ctx *Input_section_partContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInput_element(ctx *Input_elementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitToken(ctx *TokenContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitIdentifier(ctx *IdentifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDiscard_token(ctx *Discard_tokenContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitKeyword(ctx *KeywordContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitContextual_keyword(ctx *Contextual_keywordContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitLiteral(ctx *LiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitBoolean_literal(ctx *Boolean_literalContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNull_literal(ctx *Null_literalContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitOperator_or_punctuator(ctx *Operator_or_punctuatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRight_shift(ctx *Right_shiftContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRight_shift_assignment(ctx *Right_shift_assignmentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNamespace_name(ctx *Namespace_nameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitType_name(ctx *Type_nameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNamespace_or_type_name(ctx *Namespace_or_type_nameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitType_(ctx *Type_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitReference_type(ctx *Reference_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNon_nullable_reference_type(ctx *Non_nullable_reference_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitClass_type(ctx *Class_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInterface_type(ctx *Interface_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitArray_type(ctx *Array_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNon_array_type(ctx *Non_array_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRank_specifier(ctx *Rank_specifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDelegate_type(ctx *Delegate_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNullable_reference_type(ctx *Nullable_reference_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNullable_type_annotation(ctx *Nullable_type_annotationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitValue_type(ctx *Value_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNon_nullable_value_type(ctx *Non_nullable_value_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStruct_type(ctx *Struct_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSimple_type(ctx *Simple_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNumeric_type(ctx *Numeric_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitIntegral_type(ctx *Integral_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFloating_point_type(ctx *Floating_point_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitTuple_type(ctx *Tuple_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitTuple_type_element(ctx *Tuple_type_elementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitEnum_type(ctx *Enum_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNullable_value_type(ctx *Nullable_value_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitType_argument_list(ctx *Type_argument_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitType_argument(ctx *Type_argumentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitType_parameter(ctx *Type_parameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitUnmanaged_type(ctx *Unmanaged_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitVariable_reference(ctx *Variable_referenceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitPattern(ctx *PatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDeclaration_pattern(ctx *Declaration_patternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSimple_designation(ctx *Simple_designationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDiscard_designation(ctx *Discard_designationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSingle_variable_designation(ctx *Single_variable_designationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConstant_pattern(ctx *Constant_patternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitVar_pattern(ctx *Var_patternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDesignation(ctx *DesignationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitTuple_designation(ctx *Tuple_designationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDesignations(ctx *DesignationsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitPositional_pattern(ctx *Positional_patternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSubpatterns(ctx *SubpatternsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSubpattern(ctx *SubpatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitProperty_pattern(ctx *Property_patternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitProperty_subpattern(ctx *Property_subpatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDiscard_pattern(ctx *Discard_patternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitArgument_list(ctx *Argument_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitArgument(ctx *ArgumentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitArgument_name(ctx *Argument_nameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitArgument_value(ctx *Argument_valueContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitPrimary_expression(ctx *Primary_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInterpolated_string_expression(ctx *Interpolated_string_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInterpolated_regular_string_expression(ctx *Interpolated_regular_string_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRegular_interpolation(ctx *Regular_interpolationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInterpolation_minimum_width(ctx *Interpolation_minimum_widthContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInterpolated_verbatim_string_expression(ctx *Interpolated_verbatim_string_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitVerbatim_interpolation(ctx *Verbatim_interpolationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSimple_name(ctx *Simple_nameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitParenthesized_expression(ctx *Parenthesized_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitTuple_expression(ctx *Tuple_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitTuple_element(ctx *Tuple_elementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDeconstruction_expression(ctx *Deconstruction_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDeconstruction_tuple(ctx *Deconstruction_tupleContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDeconstruction_element(ctx *Deconstruction_elementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitMember_access(ctx *Member_accessContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitPredefined_type(ctx *Predefined_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNull_conditional_member_access(ctx *Null_conditional_member_accessContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDependent_access(ctx *Dependent_accessContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNull_conditional_projection_initializer(ctx *Null_conditional_projection_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNull_forgiving_operator(ctx *Null_forgiving_operatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNull_conditional_invocation_expression(ctx *Null_conditional_invocation_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNull_conditional_element_access(ctx *Null_conditional_element_accessContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitThis_access(ctx *This_accessContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitBase_access(ctx *Base_accessContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitPost_increment_expression(ctx *Post_increment_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitPost_decrement_expression(ctx *Post_decrement_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitObject_creation_expression(ctx *Object_creation_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitObject_or_collection_initializer(ctx *Object_or_collection_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitObject_initializer(ctx *Object_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitMember_initializer_list(ctx *Member_initializer_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitMember_initializer(ctx *Member_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInitializer_target(ctx *Initializer_targetContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInitializer_value(ctx *Initializer_valueContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitCollection_initializer(ctx *Collection_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitElement_initializer_list(ctx *Element_initializer_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitElement_initializer(ctx *Element_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitExpression_list(ctx *Expression_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAnonymous_object_creation_expression(ctx *Anonymous_object_creation_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAnonymous_object_initializer(ctx *Anonymous_object_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitMember_declarator_list(ctx *Member_declarator_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitMember_declarator(ctx *Member_declaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitArray_creation_expression(ctx *Array_creation_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDelegate_creation_expression(ctx *Delegate_creation_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitTypeof_expression(ctx *Typeof_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitUnbound_type_name(ctx *Unbound_type_nameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitUnbound_qualified_alias_member(ctx *Unbound_qualified_alias_memberContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitGeneric_dimension_specifier(ctx *Generic_dimension_specifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitComma(ctx *CommaContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSizeof_expression(ctx *Sizeof_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitMakeref_expression(ctx *Makeref_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitReftype_expression(ctx *Reftype_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRefvalue_expression(ctx *Refvalue_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitChecked_expression(ctx *Checked_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitUnchecked_expression(ctx *Unchecked_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDefault_value_expression(ctx *Default_value_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitExplicitly_typed_default(ctx *Explicitly_typed_defaultContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDefault_literal(ctx *Default_literalContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStackalloc_expression(ctx *Stackalloc_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStackalloc_initializer(ctx *Stackalloc_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStackalloc_initializer_element_list(ctx *Stackalloc_initializer_element_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStackalloc_element_initializer(ctx *Stackalloc_element_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNameof_expression(ctx *Nameof_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNamed_entity(ctx *Named_entityContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNamed_entity_target(ctx *Named_entity_targetContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitUnary_expression(ctx *Unary_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitPre_increment_expression(ctx *Pre_increment_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitPre_decrement_expression(ctx *Pre_decrement_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitCast_expression(ctx *Cast_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAwait_expression(ctx *Await_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRange_expression(ctx *Range_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSwitch_expression(ctx *Switch_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSwitch_expression_arms(ctx *Switch_expression_armsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSwitch_expression_arm(ctx *Switch_expression_armContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSwitch_expression_arm_expression(ctx *Switch_expression_arm_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitMultiplicative_expression(ctx *Multiplicative_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAdditive_expression(ctx *Additive_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitShift_expression(ctx *Shift_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRelational_expression(ctx *Relational_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitEquality_expression(ctx *Equality_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAnd_expression(ctx *And_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitExclusive_or_expression(ctx *Exclusive_or_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInclusive_or_expression(ctx *Inclusive_or_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConditional_and_expression(ctx *Conditional_and_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConditional_or_expression(ctx *Conditional_or_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNull_coalescing_expression(ctx *Null_coalescing_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitThrow_expression(ctx *Throw_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDeclaration_expression(ctx *Declaration_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConditional_expression(ctx *Conditional_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitLambda_expression(ctx *Lambda_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAnonymous_method_expression(ctx *Anonymous_method_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAnonymous_function_signature(ctx *Anonymous_function_signatureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitExplicit_anonymous_function_signature(ctx *Explicit_anonymous_function_signatureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitExplicit_anonymous_function_parameter_list(ctx *Explicit_anonymous_function_parameter_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitExplicit_anonymous_function_parameter(ctx *Explicit_anonymous_function_parameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAnonymous_function_parameter_modifier(ctx *Anonymous_function_parameter_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitImplicit_anonymous_function_signature(ctx *Implicit_anonymous_function_signatureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitImplicit_anonymous_function_parameter_list(ctx *Implicit_anonymous_function_parameter_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitImplicit_anonymous_function_parameter(ctx *Implicit_anonymous_function_parameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAnonymous_function_body(ctx *Anonymous_function_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitQuery_expression(ctx *Query_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFrom_clause(ctx *From_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitQuery_body(ctx *Query_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitQuery_body_clause(ctx *Query_body_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitLet_clause(ctx *Let_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitWhere_clause(ctx *Where_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitJoin_clause(ctx *Join_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitJoin_into_clause(ctx *Join_into_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitOrderby_clause(ctx *Orderby_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitOrderings(ctx *OrderingsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitOrdering(ctx *OrderingContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitOrdering_direction(ctx *Ordering_directionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSelect_or_group_clause(ctx *Select_or_group_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSelect_clause(ctx *Select_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitGroup_clause(ctx *Group_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitQuery_continuation(ctx *Query_continuationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAssignment(ctx *AssignmentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAssignment_operator(ctx *Assignment_operatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitExpression(ctx *ExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNon_assignment_expression(ctx *Non_assignment_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConstant_expression(ctx *Constant_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitBoolean_expression(ctx *Boolean_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStatement(ctx *StatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitEmbedded_statement(ctx *Embedded_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitBlock(ctx *BlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStatement_list(ctx *Statement_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitEmpty_statement(ctx *Empty_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitLabeled_statement(ctx *Labeled_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDeclaration_statement(ctx *Declaration_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitLocal_variable_declaration(ctx *Local_variable_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitLocal_variable_type(ctx *Local_variable_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitLocal_variable_declarator(ctx *Local_variable_declaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitLocal_variable_initializer(ctx *Local_variable_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitLocal_constant_declaration(ctx *Local_constant_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConstant_declarators(ctx *Constant_declaratorsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConstant_declarator(ctx *Constant_declaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitLocal_function_declaration(ctx *Local_function_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitLocal_function_header(ctx *Local_function_headerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitLocal_function_modifier(ctx *Local_function_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRef_local_function_modifier(ctx *Ref_local_function_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitLocal_function_body(ctx *Local_function_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRef_local_function_body(ctx *Ref_local_function_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitExpression_statement(ctx *Expression_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStatement_expression(ctx *Statement_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSelection_statement(ctx *Selection_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitIf_statement(ctx *If_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSwitch_statement(ctx *Switch_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSelector_expression(ctx *Selector_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSwitch_block(ctx *Switch_blockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSwitch_section(ctx *Switch_sectionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSwitch_label(ctx *Switch_labelContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitCase_guard(ctx *Case_guardContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitIteration_statement(ctx *Iteration_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitWhile_statement(ctx *While_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDo_statement(ctx *Do_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFor_statement(ctx *For_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFor_initializer(ctx *For_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFor_condition(ctx *For_conditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFor_iterator(ctx *For_iteratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStatement_expression_list(ctx *Statement_expression_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitForeach_statement(ctx *Foreach_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitJump_statement(ctx *Jump_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitBreak_statement(ctx *Break_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitContinue_statement(ctx *Continue_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitGoto_statement(ctx *Goto_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitReturn_statement(ctx *Return_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitThrow_statement(ctx *Throw_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitTry_statement(ctx *Try_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitCatch_clauses(ctx *Catch_clausesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSpecific_catch_clause(ctx *Specific_catch_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitException_specifier(ctx *Exception_specifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitException_filter(ctx *Exception_filterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitGeneral_catch_clause(ctx *General_catch_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFinally_clause(ctx *Finally_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitChecked_statement(ctx *Checked_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitUnchecked_statement(ctx *Unchecked_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitLock_statement(ctx *Lock_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitUsing_statement(ctx *Using_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitResource_acquisition(ctx *Resource_acquisitionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNon_ref_local_variable_declaration(ctx *Non_ref_local_variable_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitUsing_declaration(ctx *Using_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitYield_statement(ctx *Yield_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitCompilation_unit(ctx *Compilation_unitContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNamespace_declaration(ctx *Namespace_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitQualified_identifier(ctx *Qualified_identifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNamespace_body(ctx *Namespace_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitExtern_alias_directive(ctx *Extern_alias_directiveContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitUsing_directive(ctx *Using_directiveContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitUsing_alias_directive(ctx *Using_alias_directiveContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitUsing_namespace_directive(ctx *Using_namespace_directiveContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitUsing_static_directive(ctx *Using_static_directiveContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNamespace_member_declaration(ctx *Namespace_member_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitType_declaration(ctx *Type_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitQualified_alias_member(ctx *Qualified_alias_memberContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitClass_declaration(ctx *Class_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitClass_modifier(ctx *Class_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitType_parameter_list(ctx *Type_parameter_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDecorated_type_parameter(ctx *Decorated_type_parameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitClass_base(ctx *Class_baseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInterface_type_list(ctx *Interface_type_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitType_parameter_constraints_clause(ctx *Type_parameter_constraints_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitType_parameter_constraints(ctx *Type_parameter_constraintsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitPrimary_constraint(ctx *Primary_constraintContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSecondary_constraint(ctx *Secondary_constraintContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSecondary_constraints(ctx *Secondary_constraintsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConstructor_constraint(ctx *Constructor_constraintContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitClass_body(ctx *Class_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitClass_member_declaration(ctx *Class_member_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConstant_declaration(ctx *Constant_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConstant_modifier(ctx *Constant_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitField_declaration(ctx *Field_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitField_modifier(ctx *Field_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitVariable_declarators(ctx *Variable_declaratorsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitVariable_declarator(ctx *Variable_declaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitMethod_declaration(ctx *Method_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitMethod_modifiers(ctx *Method_modifiersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRef_kind(ctx *Ref_kindContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRef_method_modifiers(ctx *Ref_method_modifiersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitMethod_header(ctx *Method_headerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitMethod_modifier(ctx *Method_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRef_method_modifier(ctx *Ref_method_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitReturn_type(ctx *Return_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRef_return_type(ctx *Ref_return_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitMember_name(ctx *Member_nameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitMethod_body(ctx *Method_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRef_method_body(ctx *Ref_method_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitParameter_list(ctx *Parameter_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFixed_parameters(ctx *Fixed_parametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFixed_parameter(ctx *Fixed_parameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDefault_argument(ctx *Default_argumentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitParameter_modifier(ctx *Parameter_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitParameter_mode_modifier(ctx *Parameter_mode_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitParameter_array(ctx *Parameter_arrayContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitProperty_declaration(ctx *Property_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitProperty_modifier(ctx *Property_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitProperty_body(ctx *Property_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitProperty_initializer(ctx *Property_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRef_property_body(ctx *Ref_property_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAccessor_declarations(ctx *Accessor_declarationsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitGet_accessor_declaration(ctx *Get_accessor_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitSet_accessor_declaration(ctx *Set_accessor_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAccessor_modifier(ctx *Accessor_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAccessor_body(ctx *Accessor_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRef_get_accessor_declaration(ctx *Ref_get_accessor_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRef_accessor_body(ctx *Ref_accessor_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitEvent_declaration(ctx *Event_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitEvent_modifier(ctx *Event_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitEvent_accessor_declarations(ctx *Event_accessor_declarationsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAdd_accessor_declaration(ctx *Add_accessor_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRemove_accessor_declaration(ctx *Remove_accessor_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitIndexer_declaration(ctx *Indexer_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitIndexer_modifier(ctx *Indexer_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitIndexer_declarator(ctx *Indexer_declaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitIndexer_body(ctx *Indexer_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitRef_indexer_body(ctx *Ref_indexer_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitOperator_declaration(ctx *Operator_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitOperator_modifier(ctx *Operator_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitOperator_declarator(ctx *Operator_declaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitUnary_operator_declarator(ctx *Unary_operator_declaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitLogical_negation_operator(ctx *Logical_negation_operatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitOverloadable_unary_operator(ctx *Overloadable_unary_operatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitBinary_operator_declarator(ctx *Binary_operator_declaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitOverloadable_binary_operator(ctx *Overloadable_binary_operatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConversion_operator_declarator(ctx *Conversion_operator_declaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitOperator_body(ctx *Operator_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConstructor_declaration(ctx *Constructor_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConstructor_modifier(ctx *Constructor_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConstructor_declarator(ctx *Constructor_declaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConstructor_initializer(ctx *Constructor_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitConstructor_body(ctx *Constructor_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStatic_constructor_declaration(ctx *Static_constructor_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStatic_constructor_modifiers(ctx *Static_constructor_modifiersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStatic_constructor_body(ctx *Static_constructor_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFinalizer_declaration(ctx *Finalizer_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFinalizer_body(ctx *Finalizer_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStruct_declaration(ctx *Struct_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStruct_modifier(ctx *Struct_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStruct_interfaces(ctx *Struct_interfacesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStruct_body(ctx *Struct_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitStruct_member_declaration(ctx *Struct_member_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitArray_initializer(ctx *Array_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitVariable_initializer_list(ctx *Variable_initializer_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitVariable_initializer(ctx *Variable_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInterface_declaration(ctx *Interface_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInterface_modifier(ctx *Interface_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitVariant_type_parameter_list(ctx *Variant_type_parameter_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitVariant_type_parameter(ctx *Variant_type_parameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitVariance_annotation(ctx *Variance_annotationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInterface_base(ctx *Interface_baseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInterface_body(ctx *Interface_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitInterface_member_declaration(ctx *Interface_member_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitEnum_declaration(ctx *Enum_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitEnum_base(ctx *Enum_baseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitIntegral_type_name(ctx *Integral_type_nameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitEnum_body(ctx *Enum_bodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitEnum_modifier(ctx *Enum_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitEnum_member_declarations(ctx *Enum_member_declarationsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitEnum_member_declaration(ctx *Enum_member_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDelegate_declaration(ctx *Delegate_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDelegate_header(ctx *Delegate_headerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitDelegate_modifier(ctx *Delegate_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitGlobal_attributes(ctx *Global_attributesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitGlobal_attribute_section(ctx *Global_attribute_sectionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitGlobal_attribute_target_specifier(ctx *Global_attribute_target_specifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitGlobal_attribute_target(ctx *Global_attribute_targetContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAttributes(ctx *AttributesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAttribute_section(ctx *Attribute_sectionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAttribute_target_specifier(ctx *Attribute_target_specifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAttribute_target(ctx *Attribute_targetContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAttribute_list(ctx *Attribute_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAttribute(ctx *AttributeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAttribute_name(ctx *Attribute_nameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAttribute_arguments(ctx *Attribute_argumentsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitPositional_argument_list(ctx *Positional_argument_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitPositional_argument(ctx *Positional_argumentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNamed_argument_list(ctx *Named_argument_listContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitNamed_argument(ctx *Named_argumentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAttribute_argument_expression(ctx *Attribute_argument_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitUnsafe_modifier(ctx *Unsafe_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitUnsafe_statement(ctx *Unsafe_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitPointer_type(ctx *Pointer_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitPointer_indirection_expression(ctx *Pointer_indirection_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitAddressof_expression(ctx *Addressof_expressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFixed_statement(ctx *Fixed_statementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFixed_pointer_declarators(ctx *Fixed_pointer_declaratorsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFixed_pointer_declarator(ctx *Fixed_pointer_declaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFixed_pointer_initializer(ctx *Fixed_pointer_initializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFixed_size_buffer_declaration(ctx *Fixed_size_buffer_declarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFixed_size_buffer_modifier(ctx *Fixed_size_buffer_modifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitBuffer_element_type(ctx *Buffer_element_typeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFixed_size_buffer_declarators(ctx *Fixed_size_buffer_declaratorsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCSharpParserVisitor) VisitFixed_size_buffer_declarator(ctx *Fixed_size_buffer_declaratorContext) interface{} {
	return v.VisitChildren(ctx)
}
