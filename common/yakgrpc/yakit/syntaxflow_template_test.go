package yakit

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCreateRuleByTemplate(t *testing.T) {
	t.Run("template basic", func(t *testing.T) {
		in := &ypb.SyntaxFlowRuleAutoInput{
			Language:        "golang",
			RuleName:        "Golang OS Exec",
			RuleSubjects:    []string{"any() as $entry"},
			RuleSafeTests:   []string{"package main\nfunc main(){}"},
			RuleUnSafeTests: []string{"package main\nfunc main(){}"},
			RuleLevels:      []string{"high"},
		}

		out := createRuleByTemplate(in)
		fmt.Printf("%s\n", out)

		// Verify all key parts are present (rule_id is dynamic UUID, so can't do exact match)
		require.Contains(t, out, "type: audit")
		require.Contains(t, out, "level: high")
		require.Contains(t, out, "lang: golang")
		require.Contains(t, out, "title: \"Golang OS Exec\"")
		require.Contains(t, out, "risk: \"\"")
		require.Contains(t, out, "any() as $entry")
		require.Contains(t, out, "\"file://unsafe.go\": <<<UNSAFE")
		require.Contains(t, out, "package main\nfunc main(){}")
		require.Contains(t, out, "\"file://safe.go\": <<<SAFE")
		require.Contains(t, out, "alert_high: 1")

		// rule_id should be UUID-like
		re := regexp.MustCompile(`rule_id: \"[0-9a-fA-F-]{36}\"`)
		require.True(t, re.MatchString(out), "rule_id not found or invalid: %s", out)
	})

	t.Run("template muti", func(t *testing.T) {
		in := &ypb.SyntaxFlowRuleAutoInput{
			Language: "golang",
			RuleName: "Golang OS Exec",
			RuleSubjects: []string{`exec?{<fullTypeName>?{have: 'os/exec'}} as $entry
$entry.Command(* #-> as $sink) 

http.ResponseWriter as $input
$sink & $input as $high;
			`, `exec?{<fullTypeName>?{have: 'os/exec'}} as $entry
$entry.Command(* #-> as $mid) 
			`},
			RuleUnSafeTests: []string{`package main

import (
    "fmt"
    "os/exec"
	"net/http"
)

func executeCommand(userInput string) {
    cmd := exec.Command("echo", userInput)
    output, err := cmd.CombinedOutput()
    if err != nil {
        fmt.Println("Error:", err)
        return
    }
    fmt.Println(string(output))
}

func handler(w http.ResponseWriter, r *http.Request) {
	cmd := r.URL.Query().Get("cmd")
  	executeCommand(cmd)
}

func main() {
	http.HandleFunc("/", handler)
}
			`, `package main

import (
    "fmt"
    "os/exec"
	"net/http"
)

func executeCommand(userInput string) {
    cmd := exec.Command("echo", userInput)
    output, err := cmd.CombinedOutput()
    if err != nil {
        fmt.Println("Error:", err)
        return
    }
    fmt.Println(string(output))
}

func handler(w http.ResponseWriter, r *http.Request) {
	cmd := r.URL.Query().Get("cmd")
  	executeCommand(cmd)
}

func main() {
	http.HandleFunc("/", handler)
}`},
			RuleSafeTests: []string{`package main

func main() {

}`, `package main

func main() {

}`},
			RuleLevels: []string{"high", "mid"},
		}

		out := createRuleByTemplate(in)
		fmt.Printf("%s\n", out)

		// Verify key structure elements
		require.Contains(t, out, "title: \"Golang OS Exec\"")
		require.Contains(t, out, "type: audit")
		require.Contains(t, out, "level: high")
		require.Contains(t, out, "risk: \"\"")

		// Verify both subjects are present
		require.Contains(t, out, "exec?{<fullTypeName>?{have: 'os/exec'}} as $entry")
		require.Contains(t, out, "$entry.Command(* #-> as $sink)")
		require.Contains(t, out, "http.ResponseWriter as $input")
		require.Contains(t, out, "$sink & $input as $high;")
		require.Contains(t, out, "$entry.Command(* #-> as $mid)")

		// Verify two desc blocks for two subjects
		require.Contains(t, out, "lang: golang")
		require.Contains(t, out, "alert_high: 1")
		require.Contains(t, out, "\"file://unsafe.go\": <<<UNSAFE")
		require.Contains(t, out, "\"file://safe.go\": <<<SAFE")

		// Verify code snippets
		require.Contains(t, out, "func executeCommand(userInput string)")
		require.Contains(t, out, `cmd := exec.Command("echo", userInput)`)
		require.Contains(t, out, "func handler(w http.ResponseWriter, r *http.Request)")

		// rule_id should be UUID-like
		re := regexp.MustCompile(`rule_id: \"[0-9a-fA-F-]{36}\"`)
		require.True(t, re.MatchString(out), "rule_id not found or invalid")
	})
}
