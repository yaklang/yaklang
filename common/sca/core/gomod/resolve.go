package gomod

// Declaration keeps the logical requirement and its explicit replacement
// separate. Version is unknown for a local replacement unless the caller supplies
// further evidence. No paths are opened and no module graph is synthesized.
type Declaration struct {
	Requirement Requirement
	Effective   Module
	Replacement *Replacement
}

func (f *File) Declarations() []Declaration {
	byPath := make(map[string]Replacement, len(f.Replace))
	byVersion := make(map[Module]Replacement, len(f.Replace))
	for _, rep := range f.Replace {
		if rep.Old.Version == "" {
			byPath[rep.Old.Path] = rep
		} else {
			byVersion[rep.Old] = rep
		}
	}
	result := make([]Declaration, 0, len(f.Require))
	for _, req := range f.Require {
		logical := Module{req.Path, req.Version}
		d := Declaration{Requirement: req, Effective: logical}
		rep, ok := byVersion[logical]
		if !ok {
			rep, ok = byPath[req.Path]
		}
		if ok {
			repCopy := rep
			d.Replacement = &repCopy
			if rep.New.Version != "" {
				d.Effective = rep.New
			} else {
				d.Effective.Version = ""
			}
		}
		result = append(result, d)
	}
	return result
}
