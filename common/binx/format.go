package binx

import (
	"io"
)

func BinaryRead(conn io.Reader, descriptors ...*PartDescriptor) ([]ResultIf, error) {
	var results []ResultIf
	var ctx = make([]ResultIf, 0)
	var ret []ResultIf
	var err error
	for _, des := range descriptors {
		ret, _, ctx, err = read(ctx, des, conn, 0)
		if err != nil {
			return results, err
		}
		results = append(results, ret...)
	}
	return results, nil
}
