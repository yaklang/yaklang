package yso

import "testing"

// TestGenClasses test generate class object
func TestGenClasses(t *testing.T) {
	for name, _ := range YsoConfigInstance.Classes {
		// test generate class object
		var opts []GenClassOptionFun
		for _, param := range YsoConfigInstance.Classes[name].Params {
			opts = append(opts, SetClassParam(string(param.Name), "test"))
		}
		obj, err := GenerateClassWithType(name, opts...)
		if err != nil {
			t.Fatal(err)
		}

		// test convert class object to bytes
		_, err = ToBytes(obj)
		if err != nil {
			t.Fatal(err)
		}

		// test convert class object to bcel
		_, err = ToBcel(obj)
		if err != nil {
			t.Fatal(err)
		}

		// test convert class object to json
		_, err = ToJson(obj)
		if err != nil {
			t.Fatal(err)
		}
	}
}
