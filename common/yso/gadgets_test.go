package yso

import (
	"encoding/base64"
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yserx"
	"testing"
)

func TestSetMajorVersion(t *testing.T) {
	version := 50
	className := "TjftfYIA"
	got, err := GetClick1JavaObject(SetRuntimeExecEvilClass("whoami"),
		SetObfuscation(),
		SetClassName(className),
		SetMajorVersion(uint16(version)),
	)

	if err != nil {
		t.Errorf("GetClick1JavaObject() error = %v", err)
		return
	}
	g, _ := ToBytes(got)
	javaSerializables, err := yserx.ParseFromBytes(g)
	handleJavaValue := func(value *yserx.JavaFieldValue, handle func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable)) {
		if value.Type != yserx.X_FIELDVALUE {
			return
		}

		array2, ok := value.Object.(*yserx.JavaArray)
		if !ok || !array2.Bytescode {
			return
		}
		handle(nil, array2)
	}
	handleJavaField := func(f yserx.JavaSerializable, handle func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable)) {
		field, ok := f.(*yserx.JavaFieldValue)
		if !ok || field.Type != yserx.X_FIELDVALUE {
			return
		}

		array, ok := field.Object.(*yserx.JavaArray)
		if !ok {
			return
		}

		for _, value := range array.Values {
			handleJavaValue(value, handle)
		}
	}
	handleJavaClassData := func(o yserx.JavaSerializable, handle func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable)) {
		data, ok := o.(*yserx.JavaClassData)
		if !ok {
			return
		}

		for _, f := range data.Fields {
			handleJavaField(f, handle)
		}
	}
	handleJavaSerializable := func(objSer yserx.JavaSerializable, handle func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable)) {
		obj, ok := objSer.(*yserx.JavaObject)
		if !ok {
			return
		}

		for _, o := range obj.ClassData {
			handleJavaClassData(o, handle)
		}
	}
	WalkJavaSerializableObject(javaSerializables, func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable) {
		handleJavaSerializable(objSer, func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable) {
			fmt.Println(base64.StdEncoding.EncodeToString(objSer.(*yserx.JavaArray).Bytes))
			javaClass, ok := objSer.(*yserx.JavaArray)
			if ok {
				obj, err := javaclassparser.Parse(javaClass.Bytes)
				if err == nil {
					if obj.MajorVersion != uint16(version) {
						t.FailNow()
					}
				}
			}

		})

	})
}
