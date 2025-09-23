package aireact

import "context"

func (r *ReAct) invokeWriteYaklangCode(ctx context.Context, approach string) error {
	// start to write code:
	// * (optional)query code snippets
	// * write to file
	// * check syntax and lint
	// * (optional) run test cases
	// * (query document && modify to file)
	// * return

	satisfied := false
	for !satisfied {
		prompt := r.promptManager.GenerateYaklangCodeGenerateLoop()
		// query document or generate code
	}

	return nil
}
