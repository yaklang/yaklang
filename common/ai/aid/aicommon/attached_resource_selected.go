package aicommon

func init() {
	RegisterAttachedResourceDataFactory(
		AttachedResourceTypeSelected,
		func() AttachedResourceData { return &AttachedSelectedResourceData{} },
	)
}

type AttachedSelectedResourceData struct {
	Raw       string
	Selected  *AttachedCodeSelection
	PlainText string
}

func (d *AttachedSelectedResourceData) Type() string {
	return AttachedResourceTypeSelected
}

func (d *AttachedSelectedResourceData) Unmarshal(raw string) error {
	d.Raw = raw
	if sel, ok := ParseAttachedCodeSelection(&AttachedResource{Value: raw}); ok {
		d.Selected = sel
		d.PlainText = sel.Content
		return nil
	}
	d.PlainText = raw
	return nil
}

func (d *AttachedSelectedResourceData) BindLoopData(reactloop ReActLoopIF) error {
	return nil
}

func (d *AttachedSelectedResourceData) ToAttachData(reactloop ReActLoopIF) string {
	var emitter *Emitter
	if reactloop != nil {
		emitter = reactloop.GetEmitter()
	}
	if d.Selected != nil {
		return FormatAttachedCodeSelection(d.Selected, emitter)
	}
	return FormatAttachedSelectedText(d.PlainText, emitter)
}
