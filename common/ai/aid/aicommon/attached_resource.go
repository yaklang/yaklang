package aicommon

type AttachedResource struct {
	Type  string
	Key   string
	Value string
}

func NewAttachedResource(typ string, key string, value string) *AttachedResource {
	return &AttachedResource{
		Type:  typ,
		Key:   key,
		Value: value,
	}
}
