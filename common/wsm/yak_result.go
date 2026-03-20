package wsm

import "github.com/yaklang/yaklang/common/utils"

func yakResultToBytes(result interface{}) []byte {
	switch ret := result.(type) {
	case nil:
		return nil
	case []byte:
		return ret
	case string:
		return []byte(ret)
	case []interface{}:
		return flattenYakResultBytes(ret)
	default:
		return utils.InterfaceToBytes(result)
	}
}

func flattenYakResultBytes(items []interface{}) []byte {
	result := make([]byte, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case byte:
			result = append(result, v)
		case int:
			if 0 <= v && v <= 0xff {
				result = append(result, byte(v))
				continue
			}
			result = append(result, utils.InterfaceToBytes(v)...)
		case int64:
			if 0 <= v && v <= 0xff {
				result = append(result, byte(v))
				continue
			}
			result = append(result, utils.InterfaceToBytes(v)...)
		case []byte:
			result = append(result, v...)
		case string:
			result = append(result, v...)
		default:
			result = append(result, utils.InterfaceToBytes(v)...)
		}
	}
	return result
}
