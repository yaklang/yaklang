package ssaapi

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// parseRegexpLiteral parses a JavaScript regex literal string of the form "/pattern/flags"
// and returns (pattern, flags, ok).
// The SSA frontend represents `/admin/.*/i` as the const string `"/admin/.*/i"`.
func parseRegexpLiteral(s string) (pattern, flags string, ok bool) {
	if !strings.HasPrefix(s, "/") {
		return "", "", false
	}
	// Find the last '/' that is not the opening '/'
	lastSlash := strings.LastIndex(s[1:], "/")
	if lastSlash < 0 {
		return "", "", false
	}
	lastSlash++ // adjust for the offset in s[1:]
	pattern = s[1:lastSlash]
	flags = s[lastSlash+1:]
	return pattern, flags, true
}

// expandExpressPath converts an Express.js route path pattern to a regular expression string
// that can be used to check whether the regex middleware path would match it.
//
// Express string path rules:
//   - Parameters like :id are treated as a wildcard matching one path segment
//   - The path is matched case-insensitively (Express normalises before matching)
//
// We expand :param to [^/]+ so the resulting regexp can test the middleware regex.
func expandExpressPath(routePath string) string {
	// Replace Express route parameters (:name) with a segment wildcard
	paramRe := regexp.MustCompile(`:([A-Za-z_][A-Za-z0-9_]*)`)
	expanded := paramRe.ReplaceAllString(routePath, "[^/]+")
	// Treat trailing wildcard (*) as .+
	expanded = strings.ReplaceAll(expanded, "*", ".+")
	return expanded
}

// nativeCallMatchRegexpPath implements the <matchRegexpPath(target=$var)> native call.
//
// Usage in SyntaxFlow:
//
//	$caseSensitiveRegex<matchRegexpPath(target="$stringEndpoint")>  as $guarded
//
// For each regex literal in the input set ($caseSensitiveRegex, e.g. "/\/admin\/.*/"),
// the native call checks whether any string-path value in the target variable
// ($stringEndpoint, e.g. "/admin/users/:id") would be matched by the middleware
// regex ONLY when the i-flag is added (i.e. case-insensitively), but NOT when
// using the original case-sensitive regex.
//
// If such a relationship is found the original regex const value is emitted
// (so the caller can alert on it), meaning:
//   - Middleware regex (case-sensitive) would MISS the uppercase variant of the route
//   - If the regex had the i-flag it WOULD catch it
//     → Real bypass risk exists.
//
// Parameters:
//
//	target (positional 0 / named "target") — the SF variable name that holds
//	  the string-path values to test against (e.g. "$stringEndpoint").
//	  The leading "$" is optional.
var nativeCallMatchRegexpPath = sfvm.NativeCallFunc(func(
	v sfvm.Values,
	frame *sfvm.SFFrame,
	params *sfvm.NativeCallActualParams,
) (bool, sfvm.Values, error) {
	// --- 1. Resolve the target variable from the frame symbol table ---
	targetVarName := params.GetString(0, "target", "var", "against")
	if targetVarName == "" {
		return false, nil, utils.Error("matchRegexpPath: 'target' parameter is required (e.g. target=$stringEndpoint)")
	}
	// Strip leading '$' if present (either "$foo" or "foo" should work)
	targetVarName = strings.TrimPrefix(targetVarName, "$")

	targetOp, ok := frame.GetSymbolByName(targetVarName)
	if !ok || targetOp == nil {
		return false, nil, utils.Errorf("matchRegexpPath: variable '$%s' not found in current frame", targetVarName)
	}

	// --- 2. Collect all string-path values from the target variable ---
	var routePaths []string
	_ = targetOp.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}
		if val.IsConstInst() {
			s := codec.AnyToString(val.GetConstValue())
			routePaths = append(routePaths, s)
		}
		return nil
	})

	if len(routePaths) == 0 {
		return false, nil, utils.Errorf("matchRegexpPath: variable '$%s' contains no const string values", targetVarName)
	}

	// --- 3. For each regex literal in the input, check bypass possibility ---
	var results []sfvm.ValueOperator

	_ = v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}
		if !val.IsConstInst() {
			return nil
		}

		rawConst := codec.AnyToString(val.GetConstValue())
		pattern, flags, isRegexLiteral := parseRegexpLiteral(rawConst)
		if !isRegexLiteral {
			return nil
		}

		// Skip if already has i-flag (caller should have filtered these out, but be safe)
		if strings.Contains(flags, "i") {
			return nil
		}

		// Compile the case-sensitive version (original flags minus 'i')
		reSensitive, err := regexp.Compile(pattern)
		if err != nil {
			// Invalid pattern — skip silently
			return nil
		}

		// Compile the case-insensitive version (add 'i' flag via (?i) prefix)
		reInsensitive, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			return nil
		}

		// For each route path, check if:
		//   - case-insensitive regex matches it (or an uppercased variant),
		//   - but case-sensitive regex does NOT match the uppercased variant.
		// This models the attack: attacker sends uppercase URL, bypasses middleware
		// regex but hits the string-path endpoint.
		bypassed := false
		for _, route := range routePaths {
			// Expand Express parameter syntax for matching
			expanded := expandExpressPath(route)

			// Create a test string: uppercase version of the route (simulating attacker input)
			attackURL := strings.ToUpper(route)
			attackURL = expandExpressPath(attackURL)

			// The middleware regex should match the route path (it guards it)
			// Check with case-insensitive version: does the regex cover this path at all?
			if !reInsensitive.MatchString(expanded) && !reInsensitive.MatchString(attackURL) {
				// The regex doesn't match this route even case-insensitively — not related
				continue
			}

			// Now check: does the CASE-SENSITIVE regex miss the uppercased URL?
			if !reSensitive.MatchString(attackURL) {
				// YES — the attacker can use an uppercase URL to bypass the middleware
				// while still hitting the string-path endpoint.
				bypassed = true
				break
			}
		}

		if bypassed {
			for _, source := range v {
				_ = val.AppendPredecessor(source, frame.WithPredecessorContext("matchRegexpPath: bypass risk"))
			}
			results = append(results, val)
		}
		return nil
	})

	if len(results) > 0 {
		return true, sfvm.NewValues(results), nil
	}
	return false, nil, utils.Error("matchRegexpPath: no bypass relationship found")
})
