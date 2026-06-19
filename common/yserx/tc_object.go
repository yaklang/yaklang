package yserx

type JavaObject struct {
	Type        byte               `json:"type"`
	TypeVerbose string             `json:"type_verbose"`
	Class       JavaSerializable   `json:"class_desc"`
	ClassData   []JavaSerializable `json:"class_data"`
	Handle      uint64             `json:"handle"`
}

const INDENT = "  "

func (j *JavaObject) Marshal(cfg *MarshalContext) []byte {
	return cfg.JavaMarshaler.ObjectMarshaler(j, cfg)
}

// NewJavaObject 创建一个 Java 对象(TC_OBJECT)，由类描述与类数据组成，是反序列化攻击载荷的核心结构
// 在 yak 中通过 java.NewJavaObject 调用
// 参数:
//   - class: 对象所属的类描述对象
//   - classData: 零个或多个类数据(字段值与块数据)
//
// 返回值:
//   - Java 对象序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造一个 Java 对象并序列化
// desc = java.NewJavaClassDesc("com.example.Foo", []byte{0,0,0,0,0,0,0,1}, 0x02, java.NewJavaClassFields(), nil, nil)
// obj = java.NewJavaObject(desc)
// println(len(java.MarshalJavaObjects(obj)) > 0)
// ```
func NewJavaObject(class *JavaClassDesc, classData ...*JavaClassData) *JavaObject {
	obj := &JavaObject{
		TypeVerbose: tcToVerbose(TC_OBJECT),
		Type:        TC_OBJECT,
	}

	obj.Class = class
	var rest []JavaSerializable
	for _, i := range classData {
		rest = append(rest, i)
	}
	obj.ClassData = rest
	initTCType(obj)
	return obj
}
func (j *JavaObject) Bytes() []byte {
	return MarshalJavaObjects(j)
}
func (j *JavaObject) Json() (string, error) {
	jd, err := ToJson(j)
	return string(jd), err
}
