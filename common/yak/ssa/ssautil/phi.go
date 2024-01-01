package ssautil

type PhiContext[T any] struct {
	scope *ScopedVersionedTable[T]
	name  string
	phi   []*Versioned[T]
}

func (p *PhiContext[T]) AddPhi(i *Versioned[T]) {
	p.phi = append(p.phi, i)
}

// ProducePhi produce the phi for the captured variable
// note: this function will be called after the block sealed
func ProducePhi[T any](
	factory func(...T) T,
	scopes ...*ScopedVersionedTable[T]) {
	var ctxs = map[*ScopedVersionedTable[T]]map[string]*PhiContext[T]{}
	for _, v := range scopes {
		if v.IsRoot() {
			return
		}
		for _, name := range v.GetAllCapturedVariableNames() {
			capturedValue := v.parent.GetLatestVersion(name)
			if capturedValue == nil {
				continue
			}
			currentValue := v.GetLatestVersionInCurrentLexicalScope(name)
			if currentValue == nil {
				continue
			}
			targetScope := capturedValue.scope
			if _, ok := ctxs[targetScope]; !ok {
				ctxs[targetScope] = map[string]*PhiContext[T]{}
			}
			if _, ok := ctxs[targetScope][name]; !ok {
				ctxs[targetScope][name] = &PhiContext[T]{
					scope: targetScope,
					name:  name,
					phi:   []*Versioned[T]{},
				}
			}
			ctxs[targetScope][name].AddPhi(currentValue)
		}
	}

	for _, vars := range ctxs {
		for _, ctx := range vars {
			var vals = make([]T, 0, len(ctx.phi))
			for _, v := range ctx.phi {
				vals = append(vals, v.Value)
			}
			phi := factory(vals...)
			result := ctx.scope.CreateLexicalVariable(ctx.name, phi)
			result.SetPhi(true)
		}
	}
}
