package yaklib

import (
	"gopkg.in/yaml.v2"
)

var YamlExports = map[string]interface{}{
	"Marshal": yaml.Marshal,
	"Unmarshal": func(b []byte) (interface{}, error) {
		var i interface{}
		err := yaml.Unmarshal(b, &i)
		if err != nil {
			return nil, err
		}
		return i, nil
	},
	"UnmarshalStrict": func(b []byte) (interface{}, error) {
		var i interface{}
		err := yaml.UnmarshalStrict(b, &i)
		if err != nil {
			return nil, err
		}
		return i, nil
	},
}
