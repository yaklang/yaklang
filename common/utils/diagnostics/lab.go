package diagnostics

import "strings"

// Lab names one traced span (display + aggregation).
type Lab struct {
	Name      string
	Kind      string
	Text      string
	Desc      string
	StepIndex int // Track sub-step; -1 if not numbered.
}

type LabOption func(*Lab)

func NewLab(opts ...LabOption) Lab {
	var l Lab
	l.StepIndex = -1
	for _, opt := range opts {
		if opt != nil {
			opt(&l)
		}
	}
	if strings.TrimSpace(l.Text) == "" {
		l.Text = l.Name
	}
	return l
}

func LabName(name string) LabOption     { return func(l *Lab) { l.Name = strings.TrimSpace(name) } }
func LabKind(kind string) LabOption     { return func(l *Lab) { l.Kind = strings.TrimSpace(kind) } }
func LabText(text string) LabOption     { return func(l *Lab) { l.Text = strings.TrimSpace(text) } }
func LabDesc(desc string) LabOption     { return func(l *Lab) { l.Desc = strings.TrimSpace(desc) } }
func LabStepIndex(i int) LabOption      { return func(l *Lab) { l.StepIndex = i } }

func (l Lab) Key() string {
	if k := strings.TrimSpace(l.Name); k != "" {
		return k
	}
	return strings.TrimSpace(l.Text)
}

func (l Lab) Display() string {
	if t := strings.TrimSpace(l.Text); t != "" {
		return t
	}
	return strings.TrimSpace(l.Name)
}

func TrackStepLab(name string, stepIndex, stepCount int) Lab {
	opts := []LabOption{LabName(name), LabText(name)}
	if stepCount > 1 {
		opts = append(opts, LabStepIndex(stepIndex))
	}
	return NewLab(opts...)
}
