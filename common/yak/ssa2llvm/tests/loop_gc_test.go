package tests

import "testing"

func TestRuntime_LoopGC(t *testing.T) {
	code := `
func main() {
	i = 0
	for {
		if i > 5000 { break }
		a = sync.NewLock()
		i = i + 1
	}
	println(999)
}
`

	checkRunBinary(t, code, "main", map[string]string{"GCLOG": "1"}, []string{
		"999",
		"Releasing handle",
	})
}
