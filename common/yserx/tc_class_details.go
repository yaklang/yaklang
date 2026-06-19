package yserx

type JavaClassDetails struct {
	Type          byte               `json:"type"`
	TypeVerbose   string             `json:"type_verbose"`
	IsNull        bool               `json:"is_null"`
	ClassName     string             `json:"class_name"`
	SerialVersion []byte             `json:"serial_version"`
	Handle        uint64             `json:"handle"`
	DescFlag      byte               `json:"desc_flag"`
	Fields        *JavaClassFields   `json:"fields"`
	Annotations   []JavaSerializable `json:"annotations"`
	SuperClass    JavaSerializable   `json:"super_class"`

	// proxy class
	DynamicProxyClass               bool               `json:"dynamic_proxy_class"`
	DynamicProxyClassInterfaceCount int                `json:"dynamic_proxy_class_interface_count"`
	DynamicProxyAnnotation          []JavaSerializable `json:"dynamic_proxy_annotation"`
	DynamicProxyClassInterfaceNames []string           `json:"dynamic_proxy_class_interface_names"`
}

func (j *JavaClassDetails) IsJavaNull() bool {
	if j == nil {
		return true
	}

	if j.IsNull {
		return true
	}

	return false
}

func (j *JavaClassDetails) Marshal(cfg *MarshalContext) []byte {
	return cfg.JavaMarshaler.ClassDescMarshaler(j, cfg)
}

func (j *JavaClassDetails) Is_SC_WRITE_METHOD() bool {
	return (j.DescFlag & 0x01) == 0x01
}

func (j *JavaClassDetails) Is_SC_SERIALIZABLE() bool {
	return (j.DescFlag & 0x02) == 0x02
}

func (j *JavaClassDetails) Is_SC_EXTERNALIZABLE() bool {
	return (j.DescFlag & 0x04) == 0x04
}

func (j *JavaClassDetails) Is_SC_BLOCKDATA() bool {
	return (j.DescFlag & 0x08) == 0x08
}

func newJavaClassDetails() *JavaClassDetails {
	return &JavaClassDetails{
		Fields: &JavaClassFields{},
	}
}

// NewJavaClassDetails 创建 Java 类描述详情(类名、serialVersionUID、字段、父类等)
// 在 yak 中通过 java.NewJavaClassDetails 调用，是构造 ClassDesc 的底层结构
// 参数:
//   - className: 类的全限定名
//   - serialVersionUID: 序列化版本号(8 字节)
//   - Flag: 类描述标志位
//   - Fields: 字段描述集合
//   - Annotations: 类注解数据列表
//   - SuperClass: 父类描述详情，可为 nil
//
// 返回值:
//   - Java 类描述详情对象
//
// Example:
// ```
// // 该示例为示意性用法：构造类描述详情
// fields = java.NewJavaClassFields()
// details = java.NewJavaClassDetails("com.example.Foo", []byte{0,0,0,0,0,0,0,1}, 0x02, fields, nil, nil)
// println(details.ClassName)
// ```
func NewJavaClassDetails(
	className string,
	serialVersionUID []byte,
	Flag byte,
	Fields *JavaClassFields,
	Annotations []JavaSerializable,
	SuperClass *JavaClassDetails,
) *JavaClassDetails {
	details := &JavaClassDetails{
		Type:          TC_CLASSDESC,
		TypeVerbose:   tcToVerbose(TC_CLASSDESC),
		IsNull:        false,
		ClassName:     className,
		SerialVersion: serialVersionUID,
		DescFlag:      Flag,
		Fields:        Fields,
		Annotations:   Annotations,
		SuperClass:    SuperClass,
	}
	initTCType(details)
	return details
}
