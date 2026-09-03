// Code generated from ./CSharpParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package csharpparser // CSharpParser
import "github.com/yaklang/antlr/v4"

// A complete Visitor for a parse tree produced by CSharpParser.
type CSharpParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by CSharpParser#prog.
	VisitProg(ctx *ProgContext) interface{}

	// Visit a parse tree produced by CSharpParser#input.
	VisitInput(ctx *InputContext) interface{}

	// Visit a parse tree produced by CSharpParser#input_section.
	VisitInput_section(ctx *Input_sectionContext) interface{}

	// Visit a parse tree produced by CSharpParser#input_section_part.
	VisitInput_section_part(ctx *Input_section_partContext) interface{}

	// Visit a parse tree produced by CSharpParser#input_element.
	VisitInput_element(ctx *Input_elementContext) interface{}

	// Visit a parse tree produced by CSharpParser#token.
	VisitToken(ctx *TokenContext) interface{}

	// Visit a parse tree produced by CSharpParser#identifier.
	VisitIdentifier(ctx *IdentifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#discard_token.
	VisitDiscard_token(ctx *Discard_tokenContext) interface{}

	// Visit a parse tree produced by CSharpParser#keyword.
	VisitKeyword(ctx *KeywordContext) interface{}

	// Visit a parse tree produced by CSharpParser#contextual_keyword.
	VisitContextual_keyword(ctx *Contextual_keywordContext) interface{}

	// Visit a parse tree produced by CSharpParser#literal.
	VisitLiteral(ctx *LiteralContext) interface{}

	// Visit a parse tree produced by CSharpParser#boolean_literal.
	VisitBoolean_literal(ctx *Boolean_literalContext) interface{}

	// Visit a parse tree produced by CSharpParser#null_literal.
	VisitNull_literal(ctx *Null_literalContext) interface{}

	// Visit a parse tree produced by CSharpParser#operator_or_punctuator.
	VisitOperator_or_punctuator(ctx *Operator_or_punctuatorContext) interface{}

	// Visit a parse tree produced by CSharpParser#right_shift.
	VisitRight_shift(ctx *Right_shiftContext) interface{}

	// Visit a parse tree produced by CSharpParser#right_shift_assignment.
	VisitRight_shift_assignment(ctx *Right_shift_assignmentContext) interface{}

	// Visit a parse tree produced by CSharpParser#namespace_name.
	VisitNamespace_name(ctx *Namespace_nameContext) interface{}

	// Visit a parse tree produced by CSharpParser#type_name.
	VisitType_name(ctx *Type_nameContext) interface{}

	// Visit a parse tree produced by CSharpParser#namespace_or_type_name.
	VisitNamespace_or_type_name(ctx *Namespace_or_type_nameContext) interface{}

	// Visit a parse tree produced by CSharpParser#type_.
	VisitType_(ctx *Type_Context) interface{}

	// Visit a parse tree produced by CSharpParser#reference_type.
	VisitReference_type(ctx *Reference_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#non_nullable_reference_type.
	VisitNon_nullable_reference_type(ctx *Non_nullable_reference_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#class_type.
	VisitClass_type(ctx *Class_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#interface_type.
	VisitInterface_type(ctx *Interface_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#array_type.
	VisitArray_type(ctx *Array_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#non_array_type.
	VisitNon_array_type(ctx *Non_array_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#rank_specifier.
	VisitRank_specifier(ctx *Rank_specifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#delegate_type.
	VisitDelegate_type(ctx *Delegate_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#nullable_reference_type.
	VisitNullable_reference_type(ctx *Nullable_reference_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#nullable_type_annotation.
	VisitNullable_type_annotation(ctx *Nullable_type_annotationContext) interface{}

	// Visit a parse tree produced by CSharpParser#value_type.
	VisitValue_type(ctx *Value_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#non_nullable_value_type.
	VisitNon_nullable_value_type(ctx *Non_nullable_value_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#struct_type.
	VisitStruct_type(ctx *Struct_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#simple_type.
	VisitSimple_type(ctx *Simple_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#numeric_type.
	VisitNumeric_type(ctx *Numeric_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#integral_type.
	VisitIntegral_type(ctx *Integral_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#floating_point_type.
	VisitFloating_point_type(ctx *Floating_point_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#tuple_type.
	VisitTuple_type(ctx *Tuple_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#tuple_type_element.
	VisitTuple_type_element(ctx *Tuple_type_elementContext) interface{}

	// Visit a parse tree produced by CSharpParser#enum_type.
	VisitEnum_type(ctx *Enum_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#nullable_value_type.
	VisitNullable_value_type(ctx *Nullable_value_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#type_argument_list.
	VisitType_argument_list(ctx *Type_argument_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#type_argument.
	VisitType_argument(ctx *Type_argumentContext) interface{}

	// Visit a parse tree produced by CSharpParser#type_parameter.
	VisitType_parameter(ctx *Type_parameterContext) interface{}

	// Visit a parse tree produced by CSharpParser#unmanaged_type.
	VisitUnmanaged_type(ctx *Unmanaged_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#variable_reference.
	VisitVariable_reference(ctx *Variable_referenceContext) interface{}

	// Visit a parse tree produced by CSharpParser#pattern.
	VisitPattern(ctx *PatternContext) interface{}

	// Visit a parse tree produced by CSharpParser#declaration_pattern.
	VisitDeclaration_pattern(ctx *Declaration_patternContext) interface{}

	// Visit a parse tree produced by CSharpParser#simple_designation.
	VisitSimple_designation(ctx *Simple_designationContext) interface{}

	// Visit a parse tree produced by CSharpParser#discard_designation.
	VisitDiscard_designation(ctx *Discard_designationContext) interface{}

	// Visit a parse tree produced by CSharpParser#single_variable_designation.
	VisitSingle_variable_designation(ctx *Single_variable_designationContext) interface{}

	// Visit a parse tree produced by CSharpParser#constant_pattern.
	VisitConstant_pattern(ctx *Constant_patternContext) interface{}

	// Visit a parse tree produced by CSharpParser#var_pattern.
	VisitVar_pattern(ctx *Var_patternContext) interface{}

	// Visit a parse tree produced by CSharpParser#designation.
	VisitDesignation(ctx *DesignationContext) interface{}

	// Visit a parse tree produced by CSharpParser#tuple_designation.
	VisitTuple_designation(ctx *Tuple_designationContext) interface{}

	// Visit a parse tree produced by CSharpParser#designations.
	VisitDesignations(ctx *DesignationsContext) interface{}

	// Visit a parse tree produced by CSharpParser#positional_pattern.
	VisitPositional_pattern(ctx *Positional_patternContext) interface{}

	// Visit a parse tree produced by CSharpParser#subpatterns.
	VisitSubpatterns(ctx *SubpatternsContext) interface{}

	// Visit a parse tree produced by CSharpParser#subpattern.
	VisitSubpattern(ctx *SubpatternContext) interface{}

	// Visit a parse tree produced by CSharpParser#property_pattern.
	VisitProperty_pattern(ctx *Property_patternContext) interface{}

	// Visit a parse tree produced by CSharpParser#property_subpattern.
	VisitProperty_subpattern(ctx *Property_subpatternContext) interface{}

	// Visit a parse tree produced by CSharpParser#discard_pattern.
	VisitDiscard_pattern(ctx *Discard_patternContext) interface{}

	// Visit a parse tree produced by CSharpParser#argument_list.
	VisitArgument_list(ctx *Argument_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#argument.
	VisitArgument(ctx *ArgumentContext) interface{}

	// Visit a parse tree produced by CSharpParser#argument_name.
	VisitArgument_name(ctx *Argument_nameContext) interface{}

	// Visit a parse tree produced by CSharpParser#argument_value.
	VisitArgument_value(ctx *Argument_valueContext) interface{}

	// Visit a parse tree produced by CSharpParser#primary_expression.
	VisitPrimary_expression(ctx *Primary_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#interpolated_string_expression.
	VisitInterpolated_string_expression(ctx *Interpolated_string_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#interpolated_regular_string_expression.
	VisitInterpolated_regular_string_expression(ctx *Interpolated_regular_string_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#regular_interpolation.
	VisitRegular_interpolation(ctx *Regular_interpolationContext) interface{}

	// Visit a parse tree produced by CSharpParser#interpolation_minimum_width.
	VisitInterpolation_minimum_width(ctx *Interpolation_minimum_widthContext) interface{}

	// Visit a parse tree produced by CSharpParser#interpolated_verbatim_string_expression.
	VisitInterpolated_verbatim_string_expression(ctx *Interpolated_verbatim_string_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#verbatim_interpolation.
	VisitVerbatim_interpolation(ctx *Verbatim_interpolationContext) interface{}

	// Visit a parse tree produced by CSharpParser#simple_name.
	VisitSimple_name(ctx *Simple_nameContext) interface{}

	// Visit a parse tree produced by CSharpParser#parenthesized_expression.
	VisitParenthesized_expression(ctx *Parenthesized_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#tuple_expression.
	VisitTuple_expression(ctx *Tuple_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#tuple_element.
	VisitTuple_element(ctx *Tuple_elementContext) interface{}

	// Visit a parse tree produced by CSharpParser#deconstruction_expression.
	VisitDeconstruction_expression(ctx *Deconstruction_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#deconstruction_tuple.
	VisitDeconstruction_tuple(ctx *Deconstruction_tupleContext) interface{}

	// Visit a parse tree produced by CSharpParser#deconstruction_element.
	VisitDeconstruction_element(ctx *Deconstruction_elementContext) interface{}

	// Visit a parse tree produced by CSharpParser#member_access.
	VisitMember_access(ctx *Member_accessContext) interface{}

	// Visit a parse tree produced by CSharpParser#predefined_type.
	VisitPredefined_type(ctx *Predefined_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#null_conditional_member_access.
	VisitNull_conditional_member_access(ctx *Null_conditional_member_accessContext) interface{}

	// Visit a parse tree produced by CSharpParser#dependent_access.
	VisitDependent_access(ctx *Dependent_accessContext) interface{}

	// Visit a parse tree produced by CSharpParser#null_conditional_projection_initializer.
	VisitNull_conditional_projection_initializer(ctx *Null_conditional_projection_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#null_forgiving_operator.
	VisitNull_forgiving_operator(ctx *Null_forgiving_operatorContext) interface{}

	// Visit a parse tree produced by CSharpParser#null_conditional_invocation_expression.
	VisitNull_conditional_invocation_expression(ctx *Null_conditional_invocation_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#null_conditional_element_access.
	VisitNull_conditional_element_access(ctx *Null_conditional_element_accessContext) interface{}

	// Visit a parse tree produced by CSharpParser#this_access.
	VisitThis_access(ctx *This_accessContext) interface{}

	// Visit a parse tree produced by CSharpParser#base_access.
	VisitBase_access(ctx *Base_accessContext) interface{}

	// Visit a parse tree produced by CSharpParser#post_increment_expression.
	VisitPost_increment_expression(ctx *Post_increment_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#post_decrement_expression.
	VisitPost_decrement_expression(ctx *Post_decrement_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#object_creation_expression.
	VisitObject_creation_expression(ctx *Object_creation_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#object_or_collection_initializer.
	VisitObject_or_collection_initializer(ctx *Object_or_collection_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#object_initializer.
	VisitObject_initializer(ctx *Object_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#member_initializer_list.
	VisitMember_initializer_list(ctx *Member_initializer_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#member_initializer.
	VisitMember_initializer(ctx *Member_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#initializer_target.
	VisitInitializer_target(ctx *Initializer_targetContext) interface{}

	// Visit a parse tree produced by CSharpParser#initializer_value.
	VisitInitializer_value(ctx *Initializer_valueContext) interface{}

	// Visit a parse tree produced by CSharpParser#collection_initializer.
	VisitCollection_initializer(ctx *Collection_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#element_initializer_list.
	VisitElement_initializer_list(ctx *Element_initializer_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#element_initializer.
	VisitElement_initializer(ctx *Element_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#expression_list.
	VisitExpression_list(ctx *Expression_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#anonymous_object_creation_expression.
	VisitAnonymous_object_creation_expression(ctx *Anonymous_object_creation_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#anonymous_object_initializer.
	VisitAnonymous_object_initializer(ctx *Anonymous_object_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#member_declarator_list.
	VisitMember_declarator_list(ctx *Member_declarator_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#member_declarator.
	VisitMember_declarator(ctx *Member_declaratorContext) interface{}

	// Visit a parse tree produced by CSharpParser#array_creation_expression.
	VisitArray_creation_expression(ctx *Array_creation_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#delegate_creation_expression.
	VisitDelegate_creation_expression(ctx *Delegate_creation_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#typeof_expression.
	VisitTypeof_expression(ctx *Typeof_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#unbound_type_name.
	VisitUnbound_type_name(ctx *Unbound_type_nameContext) interface{}

	// Visit a parse tree produced by CSharpParser#unbound_qualified_alias_member.
	VisitUnbound_qualified_alias_member(ctx *Unbound_qualified_alias_memberContext) interface{}

	// Visit a parse tree produced by CSharpParser#generic_dimension_specifier.
	VisitGeneric_dimension_specifier(ctx *Generic_dimension_specifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#comma.
	VisitComma(ctx *CommaContext) interface{}

	// Visit a parse tree produced by CSharpParser#sizeof_expression.
	VisitSizeof_expression(ctx *Sizeof_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#makeref_expression.
	VisitMakeref_expression(ctx *Makeref_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#reftype_expression.
	VisitReftype_expression(ctx *Reftype_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#refvalue_expression.
	VisitRefvalue_expression(ctx *Refvalue_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#checked_expression.
	VisitChecked_expression(ctx *Checked_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#unchecked_expression.
	VisitUnchecked_expression(ctx *Unchecked_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#default_value_expression.
	VisitDefault_value_expression(ctx *Default_value_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#explicitly_typed_default.
	VisitExplicitly_typed_default(ctx *Explicitly_typed_defaultContext) interface{}

	// Visit a parse tree produced by CSharpParser#default_literal.
	VisitDefault_literal(ctx *Default_literalContext) interface{}

	// Visit a parse tree produced by CSharpParser#stackalloc_expression.
	VisitStackalloc_expression(ctx *Stackalloc_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#stackalloc_initializer.
	VisitStackalloc_initializer(ctx *Stackalloc_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#stackalloc_initializer_element_list.
	VisitStackalloc_initializer_element_list(ctx *Stackalloc_initializer_element_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#stackalloc_element_initializer.
	VisitStackalloc_element_initializer(ctx *Stackalloc_element_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#nameof_expression.
	VisitNameof_expression(ctx *Nameof_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#named_entity.
	VisitNamed_entity(ctx *Named_entityContext) interface{}

	// Visit a parse tree produced by CSharpParser#named_entity_target.
	VisitNamed_entity_target(ctx *Named_entity_targetContext) interface{}

	// Visit a parse tree produced by CSharpParser#unary_expression.
	VisitUnary_expression(ctx *Unary_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#pre_increment_expression.
	VisitPre_increment_expression(ctx *Pre_increment_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#pre_decrement_expression.
	VisitPre_decrement_expression(ctx *Pre_decrement_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#cast_expression.
	VisitCast_expression(ctx *Cast_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#await_expression.
	VisitAwait_expression(ctx *Await_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#range_expression.
	VisitRange_expression(ctx *Range_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#switch_expression.
	VisitSwitch_expression(ctx *Switch_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#switch_expression_arms.
	VisitSwitch_expression_arms(ctx *Switch_expression_armsContext) interface{}

	// Visit a parse tree produced by CSharpParser#switch_expression_arm.
	VisitSwitch_expression_arm(ctx *Switch_expression_armContext) interface{}

	// Visit a parse tree produced by CSharpParser#switch_expression_arm_expression.
	VisitSwitch_expression_arm_expression(ctx *Switch_expression_arm_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#multiplicative_expression.
	VisitMultiplicative_expression(ctx *Multiplicative_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#additive_expression.
	VisitAdditive_expression(ctx *Additive_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#shift_expression.
	VisitShift_expression(ctx *Shift_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#relational_expression.
	VisitRelational_expression(ctx *Relational_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#equality_expression.
	VisitEquality_expression(ctx *Equality_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#and_expression.
	VisitAnd_expression(ctx *And_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#exclusive_or_expression.
	VisitExclusive_or_expression(ctx *Exclusive_or_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#inclusive_or_expression.
	VisitInclusive_or_expression(ctx *Inclusive_or_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#conditional_and_expression.
	VisitConditional_and_expression(ctx *Conditional_and_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#conditional_or_expression.
	VisitConditional_or_expression(ctx *Conditional_or_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#null_coalescing_expression.
	VisitNull_coalescing_expression(ctx *Null_coalescing_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#throw_expression.
	VisitThrow_expression(ctx *Throw_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#declaration_expression.
	VisitDeclaration_expression(ctx *Declaration_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#conditional_expression.
	VisitConditional_expression(ctx *Conditional_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#lambda_expression.
	VisitLambda_expression(ctx *Lambda_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#anonymous_method_expression.
	VisitAnonymous_method_expression(ctx *Anonymous_method_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#anonymous_function_signature.
	VisitAnonymous_function_signature(ctx *Anonymous_function_signatureContext) interface{}

	// Visit a parse tree produced by CSharpParser#explicit_anonymous_function_signature.
	VisitExplicit_anonymous_function_signature(ctx *Explicit_anonymous_function_signatureContext) interface{}

	// Visit a parse tree produced by CSharpParser#explicit_anonymous_function_parameter_list.
	VisitExplicit_anonymous_function_parameter_list(ctx *Explicit_anonymous_function_parameter_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#explicit_anonymous_function_parameter.
	VisitExplicit_anonymous_function_parameter(ctx *Explicit_anonymous_function_parameterContext) interface{}

	// Visit a parse tree produced by CSharpParser#anonymous_function_parameter_modifier.
	VisitAnonymous_function_parameter_modifier(ctx *Anonymous_function_parameter_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#implicit_anonymous_function_signature.
	VisitImplicit_anonymous_function_signature(ctx *Implicit_anonymous_function_signatureContext) interface{}

	// Visit a parse tree produced by CSharpParser#implicit_anonymous_function_parameter_list.
	VisitImplicit_anonymous_function_parameter_list(ctx *Implicit_anonymous_function_parameter_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#implicit_anonymous_function_parameter.
	VisitImplicit_anonymous_function_parameter(ctx *Implicit_anonymous_function_parameterContext) interface{}

	// Visit a parse tree produced by CSharpParser#anonymous_function_body.
	VisitAnonymous_function_body(ctx *Anonymous_function_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#query_expression.
	VisitQuery_expression(ctx *Query_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#from_clause.
	VisitFrom_clause(ctx *From_clauseContext) interface{}

	// Visit a parse tree produced by CSharpParser#query_body.
	VisitQuery_body(ctx *Query_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#query_body_clause.
	VisitQuery_body_clause(ctx *Query_body_clauseContext) interface{}

	// Visit a parse tree produced by CSharpParser#let_clause.
	VisitLet_clause(ctx *Let_clauseContext) interface{}

	// Visit a parse tree produced by CSharpParser#where_clause.
	VisitWhere_clause(ctx *Where_clauseContext) interface{}

	// Visit a parse tree produced by CSharpParser#join_clause.
	VisitJoin_clause(ctx *Join_clauseContext) interface{}

	// Visit a parse tree produced by CSharpParser#join_into_clause.
	VisitJoin_into_clause(ctx *Join_into_clauseContext) interface{}

	// Visit a parse tree produced by CSharpParser#orderby_clause.
	VisitOrderby_clause(ctx *Orderby_clauseContext) interface{}

	// Visit a parse tree produced by CSharpParser#orderings.
	VisitOrderings(ctx *OrderingsContext) interface{}

	// Visit a parse tree produced by CSharpParser#ordering.
	VisitOrdering(ctx *OrderingContext) interface{}

	// Visit a parse tree produced by CSharpParser#ordering_direction.
	VisitOrdering_direction(ctx *Ordering_directionContext) interface{}

	// Visit a parse tree produced by CSharpParser#select_or_group_clause.
	VisitSelect_or_group_clause(ctx *Select_or_group_clauseContext) interface{}

	// Visit a parse tree produced by CSharpParser#select_clause.
	VisitSelect_clause(ctx *Select_clauseContext) interface{}

	// Visit a parse tree produced by CSharpParser#group_clause.
	VisitGroup_clause(ctx *Group_clauseContext) interface{}

	// Visit a parse tree produced by CSharpParser#query_continuation.
	VisitQuery_continuation(ctx *Query_continuationContext) interface{}

	// Visit a parse tree produced by CSharpParser#assignment.
	VisitAssignment(ctx *AssignmentContext) interface{}

	// Visit a parse tree produced by CSharpParser#assignment_operator.
	VisitAssignment_operator(ctx *Assignment_operatorContext) interface{}

	// Visit a parse tree produced by CSharpParser#expression.
	VisitExpression(ctx *ExpressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#non_assignment_expression.
	VisitNon_assignment_expression(ctx *Non_assignment_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#constant_expression.
	VisitConstant_expression(ctx *Constant_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#boolean_expression.
	VisitBoolean_expression(ctx *Boolean_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#statement.
	VisitStatement(ctx *StatementContext) interface{}

	// Visit a parse tree produced by CSharpParser#embedded_statement.
	VisitEmbedded_statement(ctx *Embedded_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#block.
	VisitBlock(ctx *BlockContext) interface{}

	// Visit a parse tree produced by CSharpParser#statement_list.
	VisitStatement_list(ctx *Statement_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#empty_statement.
	VisitEmpty_statement(ctx *Empty_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#labeled_statement.
	VisitLabeled_statement(ctx *Labeled_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#declaration_statement.
	VisitDeclaration_statement(ctx *Declaration_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#local_variable_declaration.
	VisitLocal_variable_declaration(ctx *Local_variable_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#local_variable_type.
	VisitLocal_variable_type(ctx *Local_variable_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#local_variable_declarator.
	VisitLocal_variable_declarator(ctx *Local_variable_declaratorContext) interface{}

	// Visit a parse tree produced by CSharpParser#local_variable_initializer.
	VisitLocal_variable_initializer(ctx *Local_variable_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#local_constant_declaration.
	VisitLocal_constant_declaration(ctx *Local_constant_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#constant_declarators.
	VisitConstant_declarators(ctx *Constant_declaratorsContext) interface{}

	// Visit a parse tree produced by CSharpParser#constant_declarator.
	VisitConstant_declarator(ctx *Constant_declaratorContext) interface{}

	// Visit a parse tree produced by CSharpParser#local_function_declaration.
	VisitLocal_function_declaration(ctx *Local_function_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#local_function_header.
	VisitLocal_function_header(ctx *Local_function_headerContext) interface{}

	// Visit a parse tree produced by CSharpParser#local_function_modifier.
	VisitLocal_function_modifier(ctx *Local_function_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#ref_local_function_modifier.
	VisitRef_local_function_modifier(ctx *Ref_local_function_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#local_function_body.
	VisitLocal_function_body(ctx *Local_function_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#ref_local_function_body.
	VisitRef_local_function_body(ctx *Ref_local_function_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#expression_statement.
	VisitExpression_statement(ctx *Expression_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#statement_expression.
	VisitStatement_expression(ctx *Statement_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#selection_statement.
	VisitSelection_statement(ctx *Selection_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#if_statement.
	VisitIf_statement(ctx *If_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#switch_statement.
	VisitSwitch_statement(ctx *Switch_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#selector_expression.
	VisitSelector_expression(ctx *Selector_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#switch_block.
	VisitSwitch_block(ctx *Switch_blockContext) interface{}

	// Visit a parse tree produced by CSharpParser#switch_section.
	VisitSwitch_section(ctx *Switch_sectionContext) interface{}

	// Visit a parse tree produced by CSharpParser#switch_label.
	VisitSwitch_label(ctx *Switch_labelContext) interface{}

	// Visit a parse tree produced by CSharpParser#case_guard.
	VisitCase_guard(ctx *Case_guardContext) interface{}

	// Visit a parse tree produced by CSharpParser#iteration_statement.
	VisitIteration_statement(ctx *Iteration_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#while_statement.
	VisitWhile_statement(ctx *While_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#do_statement.
	VisitDo_statement(ctx *Do_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#for_statement.
	VisitFor_statement(ctx *For_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#for_initializer.
	VisitFor_initializer(ctx *For_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#for_condition.
	VisitFor_condition(ctx *For_conditionContext) interface{}

	// Visit a parse tree produced by CSharpParser#for_iterator.
	VisitFor_iterator(ctx *For_iteratorContext) interface{}

	// Visit a parse tree produced by CSharpParser#statement_expression_list.
	VisitStatement_expression_list(ctx *Statement_expression_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#foreach_statement.
	VisitForeach_statement(ctx *Foreach_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#jump_statement.
	VisitJump_statement(ctx *Jump_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#break_statement.
	VisitBreak_statement(ctx *Break_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#continue_statement.
	VisitContinue_statement(ctx *Continue_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#goto_statement.
	VisitGoto_statement(ctx *Goto_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#return_statement.
	VisitReturn_statement(ctx *Return_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#throw_statement.
	VisitThrow_statement(ctx *Throw_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#try_statement.
	VisitTry_statement(ctx *Try_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#catch_clauses.
	VisitCatch_clauses(ctx *Catch_clausesContext) interface{}

	// Visit a parse tree produced by CSharpParser#specific_catch_clause.
	VisitSpecific_catch_clause(ctx *Specific_catch_clauseContext) interface{}

	// Visit a parse tree produced by CSharpParser#exception_specifier.
	VisitException_specifier(ctx *Exception_specifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#exception_filter.
	VisitException_filter(ctx *Exception_filterContext) interface{}

	// Visit a parse tree produced by CSharpParser#general_catch_clause.
	VisitGeneral_catch_clause(ctx *General_catch_clauseContext) interface{}

	// Visit a parse tree produced by CSharpParser#finally_clause.
	VisitFinally_clause(ctx *Finally_clauseContext) interface{}

	// Visit a parse tree produced by CSharpParser#checked_statement.
	VisitChecked_statement(ctx *Checked_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#unchecked_statement.
	VisitUnchecked_statement(ctx *Unchecked_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#lock_statement.
	VisitLock_statement(ctx *Lock_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#using_statement.
	VisitUsing_statement(ctx *Using_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#resource_acquisition.
	VisitResource_acquisition(ctx *Resource_acquisitionContext) interface{}

	// Visit a parse tree produced by CSharpParser#non_ref_local_variable_declaration.
	VisitNon_ref_local_variable_declaration(ctx *Non_ref_local_variable_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#using_declaration.
	VisitUsing_declaration(ctx *Using_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#yield_statement.
	VisitYield_statement(ctx *Yield_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#compilation_unit.
	VisitCompilation_unit(ctx *Compilation_unitContext) interface{}

	// Visit a parse tree produced by CSharpParser#namespace_declaration.
	VisitNamespace_declaration(ctx *Namespace_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#qualified_identifier.
	VisitQualified_identifier(ctx *Qualified_identifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#namespace_body.
	VisitNamespace_body(ctx *Namespace_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#extern_alias_directive.
	VisitExtern_alias_directive(ctx *Extern_alias_directiveContext) interface{}

	// Visit a parse tree produced by CSharpParser#using_directive.
	VisitUsing_directive(ctx *Using_directiveContext) interface{}

	// Visit a parse tree produced by CSharpParser#using_alias_directive.
	VisitUsing_alias_directive(ctx *Using_alias_directiveContext) interface{}

	// Visit a parse tree produced by CSharpParser#using_namespace_directive.
	VisitUsing_namespace_directive(ctx *Using_namespace_directiveContext) interface{}

	// Visit a parse tree produced by CSharpParser#using_static_directive.
	VisitUsing_static_directive(ctx *Using_static_directiveContext) interface{}

	// Visit a parse tree produced by CSharpParser#namespace_member_declaration.
	VisitNamespace_member_declaration(ctx *Namespace_member_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#type_declaration.
	VisitType_declaration(ctx *Type_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#qualified_alias_member.
	VisitQualified_alias_member(ctx *Qualified_alias_memberContext) interface{}

	// Visit a parse tree produced by CSharpParser#class_declaration.
	VisitClass_declaration(ctx *Class_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#class_modifier.
	VisitClass_modifier(ctx *Class_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#type_parameter_list.
	VisitType_parameter_list(ctx *Type_parameter_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#decorated_type_parameter.
	VisitDecorated_type_parameter(ctx *Decorated_type_parameterContext) interface{}

	// Visit a parse tree produced by CSharpParser#class_base.
	VisitClass_base(ctx *Class_baseContext) interface{}

	// Visit a parse tree produced by CSharpParser#interface_type_list.
	VisitInterface_type_list(ctx *Interface_type_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#type_parameter_constraints_clause.
	VisitType_parameter_constraints_clause(ctx *Type_parameter_constraints_clauseContext) interface{}

	// Visit a parse tree produced by CSharpParser#type_parameter_constraints.
	VisitType_parameter_constraints(ctx *Type_parameter_constraintsContext) interface{}

	// Visit a parse tree produced by CSharpParser#primary_constraint.
	VisitPrimary_constraint(ctx *Primary_constraintContext) interface{}

	// Visit a parse tree produced by CSharpParser#secondary_constraint.
	VisitSecondary_constraint(ctx *Secondary_constraintContext) interface{}

	// Visit a parse tree produced by CSharpParser#secondary_constraints.
	VisitSecondary_constraints(ctx *Secondary_constraintsContext) interface{}

	// Visit a parse tree produced by CSharpParser#constructor_constraint.
	VisitConstructor_constraint(ctx *Constructor_constraintContext) interface{}

	// Visit a parse tree produced by CSharpParser#class_body.
	VisitClass_body(ctx *Class_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#class_member_declaration.
	VisitClass_member_declaration(ctx *Class_member_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#constant_declaration.
	VisitConstant_declaration(ctx *Constant_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#constant_modifier.
	VisitConstant_modifier(ctx *Constant_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#field_declaration.
	VisitField_declaration(ctx *Field_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#field_modifier.
	VisitField_modifier(ctx *Field_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#variable_declarators.
	VisitVariable_declarators(ctx *Variable_declaratorsContext) interface{}

	// Visit a parse tree produced by CSharpParser#variable_declarator.
	VisitVariable_declarator(ctx *Variable_declaratorContext) interface{}

	// Visit a parse tree produced by CSharpParser#method_declaration.
	VisitMethod_declaration(ctx *Method_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#method_modifiers.
	VisitMethod_modifiers(ctx *Method_modifiersContext) interface{}

	// Visit a parse tree produced by CSharpParser#ref_kind.
	VisitRef_kind(ctx *Ref_kindContext) interface{}

	// Visit a parse tree produced by CSharpParser#ref_method_modifiers.
	VisitRef_method_modifiers(ctx *Ref_method_modifiersContext) interface{}

	// Visit a parse tree produced by CSharpParser#method_header.
	VisitMethod_header(ctx *Method_headerContext) interface{}

	// Visit a parse tree produced by CSharpParser#method_modifier.
	VisitMethod_modifier(ctx *Method_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#ref_method_modifier.
	VisitRef_method_modifier(ctx *Ref_method_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#return_type.
	VisitReturn_type(ctx *Return_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#ref_return_type.
	VisitRef_return_type(ctx *Ref_return_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#member_name.
	VisitMember_name(ctx *Member_nameContext) interface{}

	// Visit a parse tree produced by CSharpParser#method_body.
	VisitMethod_body(ctx *Method_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#ref_method_body.
	VisitRef_method_body(ctx *Ref_method_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#parameter_list.
	VisitParameter_list(ctx *Parameter_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#fixed_parameters.
	VisitFixed_parameters(ctx *Fixed_parametersContext) interface{}

	// Visit a parse tree produced by CSharpParser#fixed_parameter.
	VisitFixed_parameter(ctx *Fixed_parameterContext) interface{}

	// Visit a parse tree produced by CSharpParser#default_argument.
	VisitDefault_argument(ctx *Default_argumentContext) interface{}

	// Visit a parse tree produced by CSharpParser#parameter_modifier.
	VisitParameter_modifier(ctx *Parameter_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#parameter_mode_modifier.
	VisitParameter_mode_modifier(ctx *Parameter_mode_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#parameter_array.
	VisitParameter_array(ctx *Parameter_arrayContext) interface{}

	// Visit a parse tree produced by CSharpParser#property_declaration.
	VisitProperty_declaration(ctx *Property_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#property_modifier.
	VisitProperty_modifier(ctx *Property_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#property_body.
	VisitProperty_body(ctx *Property_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#property_initializer.
	VisitProperty_initializer(ctx *Property_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#ref_property_body.
	VisitRef_property_body(ctx *Ref_property_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#accessor_declarations.
	VisitAccessor_declarations(ctx *Accessor_declarationsContext) interface{}

	// Visit a parse tree produced by CSharpParser#get_accessor_declaration.
	VisitGet_accessor_declaration(ctx *Get_accessor_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#set_accessor_declaration.
	VisitSet_accessor_declaration(ctx *Set_accessor_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#accessor_modifier.
	VisitAccessor_modifier(ctx *Accessor_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#accessor_body.
	VisitAccessor_body(ctx *Accessor_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#ref_get_accessor_declaration.
	VisitRef_get_accessor_declaration(ctx *Ref_get_accessor_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#ref_accessor_body.
	VisitRef_accessor_body(ctx *Ref_accessor_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#event_declaration.
	VisitEvent_declaration(ctx *Event_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#event_modifier.
	VisitEvent_modifier(ctx *Event_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#event_accessor_declarations.
	VisitEvent_accessor_declarations(ctx *Event_accessor_declarationsContext) interface{}

	// Visit a parse tree produced by CSharpParser#add_accessor_declaration.
	VisitAdd_accessor_declaration(ctx *Add_accessor_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#remove_accessor_declaration.
	VisitRemove_accessor_declaration(ctx *Remove_accessor_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#indexer_declaration.
	VisitIndexer_declaration(ctx *Indexer_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#indexer_modifier.
	VisitIndexer_modifier(ctx *Indexer_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#indexer_declarator.
	VisitIndexer_declarator(ctx *Indexer_declaratorContext) interface{}

	// Visit a parse tree produced by CSharpParser#indexer_body.
	VisitIndexer_body(ctx *Indexer_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#ref_indexer_body.
	VisitRef_indexer_body(ctx *Ref_indexer_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#operator_declaration.
	VisitOperator_declaration(ctx *Operator_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#operator_modifier.
	VisitOperator_modifier(ctx *Operator_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#operator_declarator.
	VisitOperator_declarator(ctx *Operator_declaratorContext) interface{}

	// Visit a parse tree produced by CSharpParser#unary_operator_declarator.
	VisitUnary_operator_declarator(ctx *Unary_operator_declaratorContext) interface{}

	// Visit a parse tree produced by CSharpParser#logical_negation_operator.
	VisitLogical_negation_operator(ctx *Logical_negation_operatorContext) interface{}

	// Visit a parse tree produced by CSharpParser#overloadable_unary_operator.
	VisitOverloadable_unary_operator(ctx *Overloadable_unary_operatorContext) interface{}

	// Visit a parse tree produced by CSharpParser#binary_operator_declarator.
	VisitBinary_operator_declarator(ctx *Binary_operator_declaratorContext) interface{}

	// Visit a parse tree produced by CSharpParser#overloadable_binary_operator.
	VisitOverloadable_binary_operator(ctx *Overloadable_binary_operatorContext) interface{}

	// Visit a parse tree produced by CSharpParser#conversion_operator_declarator.
	VisitConversion_operator_declarator(ctx *Conversion_operator_declaratorContext) interface{}

	// Visit a parse tree produced by CSharpParser#operator_body.
	VisitOperator_body(ctx *Operator_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#constructor_declaration.
	VisitConstructor_declaration(ctx *Constructor_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#constructor_modifier.
	VisitConstructor_modifier(ctx *Constructor_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#constructor_declarator.
	VisitConstructor_declarator(ctx *Constructor_declaratorContext) interface{}

	// Visit a parse tree produced by CSharpParser#constructor_initializer.
	VisitConstructor_initializer(ctx *Constructor_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#constructor_body.
	VisitConstructor_body(ctx *Constructor_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#static_constructor_declaration.
	VisitStatic_constructor_declaration(ctx *Static_constructor_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#static_constructor_modifiers.
	VisitStatic_constructor_modifiers(ctx *Static_constructor_modifiersContext) interface{}

	// Visit a parse tree produced by CSharpParser#static_constructor_body.
	VisitStatic_constructor_body(ctx *Static_constructor_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#finalizer_declaration.
	VisitFinalizer_declaration(ctx *Finalizer_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#finalizer_body.
	VisitFinalizer_body(ctx *Finalizer_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#struct_declaration.
	VisitStruct_declaration(ctx *Struct_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#struct_modifier.
	VisitStruct_modifier(ctx *Struct_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#struct_interfaces.
	VisitStruct_interfaces(ctx *Struct_interfacesContext) interface{}

	// Visit a parse tree produced by CSharpParser#struct_body.
	VisitStruct_body(ctx *Struct_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#struct_member_declaration.
	VisitStruct_member_declaration(ctx *Struct_member_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#array_initializer.
	VisitArray_initializer(ctx *Array_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#variable_initializer_list.
	VisitVariable_initializer_list(ctx *Variable_initializer_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#variable_initializer.
	VisitVariable_initializer(ctx *Variable_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#interface_declaration.
	VisitInterface_declaration(ctx *Interface_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#interface_modifier.
	VisitInterface_modifier(ctx *Interface_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#variant_type_parameter_list.
	VisitVariant_type_parameter_list(ctx *Variant_type_parameter_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#variant_type_parameter.
	VisitVariant_type_parameter(ctx *Variant_type_parameterContext) interface{}

	// Visit a parse tree produced by CSharpParser#variance_annotation.
	VisitVariance_annotation(ctx *Variance_annotationContext) interface{}

	// Visit a parse tree produced by CSharpParser#interface_base.
	VisitInterface_base(ctx *Interface_baseContext) interface{}

	// Visit a parse tree produced by CSharpParser#interface_body.
	VisitInterface_body(ctx *Interface_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#interface_member_declaration.
	VisitInterface_member_declaration(ctx *Interface_member_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#enum_declaration.
	VisitEnum_declaration(ctx *Enum_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#enum_base.
	VisitEnum_base(ctx *Enum_baseContext) interface{}

	// Visit a parse tree produced by CSharpParser#integral_type_name.
	VisitIntegral_type_name(ctx *Integral_type_nameContext) interface{}

	// Visit a parse tree produced by CSharpParser#enum_body.
	VisitEnum_body(ctx *Enum_bodyContext) interface{}

	// Visit a parse tree produced by CSharpParser#enum_modifier.
	VisitEnum_modifier(ctx *Enum_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#enum_member_declarations.
	VisitEnum_member_declarations(ctx *Enum_member_declarationsContext) interface{}

	// Visit a parse tree produced by CSharpParser#enum_member_declaration.
	VisitEnum_member_declaration(ctx *Enum_member_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#delegate_declaration.
	VisitDelegate_declaration(ctx *Delegate_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#delegate_header.
	VisitDelegate_header(ctx *Delegate_headerContext) interface{}

	// Visit a parse tree produced by CSharpParser#delegate_modifier.
	VisitDelegate_modifier(ctx *Delegate_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#global_attributes.
	VisitGlobal_attributes(ctx *Global_attributesContext) interface{}

	// Visit a parse tree produced by CSharpParser#global_attribute_section.
	VisitGlobal_attribute_section(ctx *Global_attribute_sectionContext) interface{}

	// Visit a parse tree produced by CSharpParser#global_attribute_target_specifier.
	VisitGlobal_attribute_target_specifier(ctx *Global_attribute_target_specifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#global_attribute_target.
	VisitGlobal_attribute_target(ctx *Global_attribute_targetContext) interface{}

	// Visit a parse tree produced by CSharpParser#attributes.
	VisitAttributes(ctx *AttributesContext) interface{}

	// Visit a parse tree produced by CSharpParser#attribute_section.
	VisitAttribute_section(ctx *Attribute_sectionContext) interface{}

	// Visit a parse tree produced by CSharpParser#attribute_target_specifier.
	VisitAttribute_target_specifier(ctx *Attribute_target_specifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#attribute_target.
	VisitAttribute_target(ctx *Attribute_targetContext) interface{}

	// Visit a parse tree produced by CSharpParser#attribute_list.
	VisitAttribute_list(ctx *Attribute_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#attribute.
	VisitAttribute(ctx *AttributeContext) interface{}

	// Visit a parse tree produced by CSharpParser#attribute_name.
	VisitAttribute_name(ctx *Attribute_nameContext) interface{}

	// Visit a parse tree produced by CSharpParser#attribute_arguments.
	VisitAttribute_arguments(ctx *Attribute_argumentsContext) interface{}

	// Visit a parse tree produced by CSharpParser#positional_argument_list.
	VisitPositional_argument_list(ctx *Positional_argument_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#positional_argument.
	VisitPositional_argument(ctx *Positional_argumentContext) interface{}

	// Visit a parse tree produced by CSharpParser#named_argument_list.
	VisitNamed_argument_list(ctx *Named_argument_listContext) interface{}

	// Visit a parse tree produced by CSharpParser#named_argument.
	VisitNamed_argument(ctx *Named_argumentContext) interface{}

	// Visit a parse tree produced by CSharpParser#attribute_argument_expression.
	VisitAttribute_argument_expression(ctx *Attribute_argument_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#unsafe_modifier.
	VisitUnsafe_modifier(ctx *Unsafe_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#unsafe_statement.
	VisitUnsafe_statement(ctx *Unsafe_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#pointer_type.
	VisitPointer_type(ctx *Pointer_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#pointer_indirection_expression.
	VisitPointer_indirection_expression(ctx *Pointer_indirection_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#addressof_expression.
	VisitAddressof_expression(ctx *Addressof_expressionContext) interface{}

	// Visit a parse tree produced by CSharpParser#fixed_statement.
	VisitFixed_statement(ctx *Fixed_statementContext) interface{}

	// Visit a parse tree produced by CSharpParser#fixed_pointer_declarators.
	VisitFixed_pointer_declarators(ctx *Fixed_pointer_declaratorsContext) interface{}

	// Visit a parse tree produced by CSharpParser#fixed_pointer_declarator.
	VisitFixed_pointer_declarator(ctx *Fixed_pointer_declaratorContext) interface{}

	// Visit a parse tree produced by CSharpParser#fixed_pointer_initializer.
	VisitFixed_pointer_initializer(ctx *Fixed_pointer_initializerContext) interface{}

	// Visit a parse tree produced by CSharpParser#fixed_size_buffer_declaration.
	VisitFixed_size_buffer_declaration(ctx *Fixed_size_buffer_declarationContext) interface{}

	// Visit a parse tree produced by CSharpParser#fixed_size_buffer_modifier.
	VisitFixed_size_buffer_modifier(ctx *Fixed_size_buffer_modifierContext) interface{}

	// Visit a parse tree produced by CSharpParser#buffer_element_type.
	VisitBuffer_element_type(ctx *Buffer_element_typeContext) interface{}

	// Visit a parse tree produced by CSharpParser#fixed_size_buffer_declarators.
	VisitFixed_size_buffer_declarators(ctx *Fixed_size_buffer_declaratorsContext) interface{}

	// Visit a parse tree produced by CSharpParser#fixed_size_buffer_declarator.
	VisitFixed_size_buffer_declarator(ctx *Fixed_size_buffer_declaratorContext) interface{}
}
