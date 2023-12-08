package yso

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yserx"
	"testing"
)

func TestMUSTPASSSetMajorVersion(t *testing.T) {
	type testCase struct {
		version     uint16
		className   string
		wantVersion uint16
	}
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

	tests := []testCase{
		{version: 50, className: "TjftfYIA", wantVersion: 50},
		{version: 51, className: "TjftfYIA", wantVersion: 51},
		{version: 133, className: "TjftfYIA", wantVersion: 52},
		// Add more test cases as needed
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("Version%d", tc.version), func(t *testing.T) {
			got, err := GetClick1JavaObject(
				SetRuntimeExecEvilClass("whoami"),
				SetObfuscation(),
				SetClassName(tc.className),
				SetMajorVersion(tc.version),
			)

			if err != nil {
				t.Errorf("GetClick1JavaObject() error = %v", err)
				return
			}
			g, _ := ToBytes(got)
			javaSerializables, err := yserx.ParseFromBytes(g)
			if err != nil {
				t.Errorf("ParseFromBytes() error = %v", err)
				return
			}

			found := false
			WalkJavaSerializableObject(javaSerializables, func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable) {
				// Assuming the WalkJavaSerializableObject and other functions are defined elsewhere
				handleJavaSerializable(objSer, func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable) {
					javaClass, ok := objSer.(*yserx.JavaArray)
					if ok {
						obj, err := javaclassparser.Parse(javaClass.Bytes)
						if err == nil && obj.MajorVersion == tc.wantVersion {
							found = true
						}
					}
				})
			})

			if !found {
				t.Errorf("Test case with version %d failed, expected major version was %d", tc.version, tc.wantVersion)
			}
		})
	}
}
