package aidmock

var HelloWorldScenario = NewKeywordScenarios()

func init() {
	HelloWorldScenario.AddResponseWithMatcher("hello_world", func(prompt string) bool {
		// return strings.Contains(prompt, "Hello, world!")
		return true
	}, BuildDirectlyAnswer("Hello, world!", "Hello world response", "Hello world response"), "Hello world response")
}
