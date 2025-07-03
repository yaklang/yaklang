package schema

const (
	// "syntaxflow" | "nuclei" | "mitm" | "port-scan" | "codec" | "yak"
	SCRIPT_TYPE_SYNTAXFLOW string = "syntaxflow"
	SCRIPT_TYPE_NASL       string = "nasl"
	SCRIPT_TYPE_NUCLEI     string = "nuclei"
	SCRIPT_TYPE_MITM       string = "mitm"
	SCRIPT_TYPE_PORT_SCAN  string = "port-scan"
	SCRIPT_TYPE_CODEC      string = "codec"
	SCRIPT_TYPE_YAK        string = "yak"
)

type ScriptOrRule interface {
	GetScriptName() string
	GetContent() string
	GetType() string
}

var _ ScriptOrRule = (*YakScript)(nil)
var _ ScriptOrRule = (*NaslScript)(nil)
var _ ScriptOrRule = (*SyntaxFlowRule)(nil)

func (s *YakScript) GetScriptName() string {
	return s.ScriptName
}
func (s *YakScript) GetContent() string {
	return s.Content
}
func (s *YakScript) GetType() string {
	return s.Type
}

func (s *NaslScript) GetScriptName() string {
	return s.ScriptName
}
func (s *NaslScript) GetContent() string {
	return s.Script
}
func (s *NaslScript) GetType() string {
	return SCRIPT_TYPE_NASL
}

func (s *SyntaxFlowRule) GetScriptName() string {
	return s.RuleName
}
func (s *SyntaxFlowRule) GetContent() string {
	return s.Content
}
func (s *SyntaxFlowRule) GetType() string {
	return SCRIPT_TYPE_SYNTAXFLOW
}
