package loop_ssa_api_discovery

import (
	_ "embed"
	"strings"
)

//go:embed prompts/http_builtin_tool_params.txt
var ssaDiscoveryHTTPBuiltinToolParamsCore string

//go:embed prompts/auth_credential_transform_hint.txt
var ssaDiscoveryAuthCredentialTransformHint string

var ssaDiscoveryHTTPBuiltinToolParamsHint = strings.TrimSpace(ssaDiscoveryHTTPBuiltinToolParamsCore) + "\n\n" + strings.TrimSpace(ssaDiscoveryAuthCredentialTransformHint)
