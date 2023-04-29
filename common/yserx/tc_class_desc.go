package yserx

// group
type JavaClassDesc struct {
	Type        byte              `json:"type"`
	TypeVerbose string            `json:"type_verbose"`
	Detail      *JavaClassDetails `json:"detail"`
	//LastDetails  *JavaClassDetails            `json:"-"`
	Map map[uint64]*JavaClassDetails `json:"-"`
}

func (j *JavaClassDesc) Marshal() []byte {
	return j.Detail.Marshal()
	//var raw []byte
	//for _, i := range j.Items {
	//	raw = append(raw, i.MarshalJavaObjects()...)
	//}
	//return raw
}

func (g *JavaClassDesc) SetDetails(d *JavaClassDetails) {
	g.Detail = d
	//g.Items = append(g.Items, d)
	//g.LastDetails = d
	if g.Map == nil {
		g.Map = map[uint64]*JavaClassDetails{}
	}
	g.Map[d.Handle] = d
}

func NewJavaClassDesc(
	className string,
	serialVersionUID []byte,
	flag byte,
	fields *JavaClassFields,
	annotations []JavaSerializable,
	superClass *JavaClassDetails,
) *JavaClassDesc {
	desc := &JavaClassDesc{}
	details := NewJavaClassDetails(
		className, serialVersionUID, flag, fields, annotations, superClass,
	)
	desc.SetDetails(details)
	return desc
}
