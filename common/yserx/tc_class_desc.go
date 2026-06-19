package yserx

// group
type JavaClassDesc struct {
	Type        byte              `json:"type"`
	TypeVerbose string            `json:"type_verbose"`
	Detail      *JavaClassDetails `json:"detail"`
	//LastDetails  *JavaClassDetails            `json:"-"`
	Map map[uint64]*JavaClassDetails `json:"-"`
}

func (j *JavaClassDesc) Marshal(cfg *MarshalContext) []byte {
	return j.Detail.Marshal(cfg)
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

// NewJavaClassDesc 创建一个 Java 类描述对象(TC_CLASSDESC)，用于描述对象所属类的元信息
// 在 yak 中通过 java.NewJavaClassDesc 调用
// 参数:
//   - className: 类的全限定名
//   - serialVersionUID: 序列化版本号(8 字节)
//   - flag: 类描述标志位
//   - fields: 字段描述集合
//   - annotations: 类注解数据列表
//   - superClass: 父类描述详情，可为 nil
//
// 返回值:
//   - Java 类描述序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造类描述
// fields = java.NewJavaClassFields()
// desc = java.NewJavaClassDesc("com.example.Foo", []byte{0,0,0,0,0,0,0,1}, 0x02, fields, nil, nil)
// obj = java.NewJavaObject(desc)
// println(len(java.MarshalJavaObjects(obj)) > 0)
// ```
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
