package yso

import "testing"

// TestGenClasses test generate class object
func TestGenClasses(t *testing.T) {
	genCfg := &ClassGenConfig{}

	for name, _ := range YsoConfigInstance.Classes {
		genCfg.ClassType = name

		// test generate class object
		obj, err := genCfg.GenerateClassObject()
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
