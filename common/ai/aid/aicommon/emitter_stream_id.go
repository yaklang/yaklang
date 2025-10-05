package aicommon

type i18n struct {
	Zh string
	En string
}

var nodeIdMapper = map[string]i18n{
	"re-act-loop-thought": {
		Zh: "思考",
		En: "Thought",
	},
}
