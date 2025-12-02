package facades

import (
	"errors"

	"github.com/yaklang/yaklang/common/yserx"
	"github.com/yaklang/yaklang/common/yso/resources"
)

func LoadReferenceResourceForRmi() (*yserx.JavaObject, error) {
	referenceData, err := resources.YsoResourceFS.ReadFile("static/gadgets/reference_for_rmi.ser")
	if err != nil {
		return nil, err
	}
	objInsList, err := yserx.ParseJavaSerialized(referenceData)
	if err != nil {
		return nil, err
	}
	if len(objInsList) == 0 || objInsList[0] == nil {
		return nil, err
	}
	res, ok := objInsList[0].(*yserx.JavaObject)
	if !ok {
		return nil, errors.New("invalid reference object")
	}
	return res, nil
}
