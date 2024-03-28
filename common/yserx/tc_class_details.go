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
