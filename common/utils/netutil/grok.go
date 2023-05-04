package netutil

import "github.com/vjeantet/grok"

type GrokResult map[string][]string

func (g GrokResult) Get(key string) string {
	res := g.GetAll(key)
	if len(res) > 0 {
		return res[0]
	}
	return ""
}

func (g GrokResult) GetAll(key string) []string {
	if g == nil {
		return nil
	}

	res, ok := g[key]
	if !ok {
		return nil
	}

	if res == nil {
		return nil
	}

	return res
}

func (g GrokResult) GetOr(key string, value string) string {
	if g.Get(key) == "" {
		return value
	}
	return g.Get(key)
}

var (
	grokParser *grok.Grok
)

func init() {
	if grokParser != nil {
		return
	}
	var err error
	grokParser, err = getGrokParser()
	if err != nil {
		panic("BUG: get grok parser failed: " + err.Error())
	}
}

func getGrokParser() (*grok.Grok, error) {
	parser, err := grok.NewWithConfig(&grok.Config{
		NamedCapturesOnly:   false,
		SkipDefaultPatterns: false,
		RemoveEmptyValues:   true,
	})
	if err != nil {
		return nil, err
	}

	err = parser.AddPatternsFromMap(map[string]string{
		`COMMONVERSION`: `(%{INT}\.?)+[a-zA-Z]*?`,
	})
	if err != nil {
		return nil, err
		//panic(fmt.Sprintf("add grok pattern failed: %s", err))
	}

	return parser, nil
}

func Grok(line string, rule string) GrokResult {
	results, err := grokParser.ParseToMultiMap(rule, line)
	if err != nil {
		return nil
	}
	return results
}
