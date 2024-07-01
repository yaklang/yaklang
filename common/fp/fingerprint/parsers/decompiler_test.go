package parsers

//func convert(exp string) string {
//	rules, err := ParseExpRule(newTestGenerateRule(exp))
//	if err != nil {
//		panic(err)
//	}
//	rule, err := rule.DecompileFingerprintRuleOpCodes(rules[0])
//	return rule.MatchExpression
//}
//
//func _TestDecompiler(t *testing.T) {
//	assert.Equal(t, `header = "MiniCMS" and (body = "1" or body = "2" or body = "2") && body = "a"`, convert(`header = "MiniCMS" && (body = "1" || body = "2" || body = "2") && body="a"`))
//	assert.Equal(t, `header = "MiniCMS" and (body = "1" or body = "2" or body = "2")`, convert(`header = "MiniCMS" && (body = "1" || body = "2" || body = "2")`))
//	assert.Equal(t, `header = "MiniCMS" and (body = "1" or body = "2")`, convert(`header = "MiniCMS" && (body = "1" || body = "2")`))
//	assert.Equal(t, `header = "\"MiniCMS\""`, convert(`header="\"MiniCMS\""`))
//}
