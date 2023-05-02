package yaklib

import (
	"bufio"
	"io"
	"reflect"
	"github.com/yaklang/yaklang/common/utils"
)

func _newReader(i interface{}) (*bufio.Reader, error) {
	if rd, ok := i.(io.Reader); ok {
		return bufio.NewReader(rd), nil
	} else {
		return nil, utils.Errorf("not support type: %v", reflect.TypeOf(i))
	}
}

func _newReaderSize(i interface{}, size int) (*bufio.Reader, error) {
	if rd, ok := i.(io.Reader); ok {
		return bufio.NewReaderSize(rd, size), nil
	} else {
		return nil, utils.Errorf("not support type: %v", reflect.TypeOf(i))
	}
}

func _newWriter(i interface{}) (*bufio.Writer, error) {
	if wd, ok := i.(io.Writer); ok {
		return bufio.NewWriter(wd), nil
	} else {
		return nil, utils.Errorf("not support type: %v", reflect.TypeOf(i))
	}
}

func _newWriterSize(i interface{}, size int) (*bufio.Writer, error) {
	if wd, ok := i.(io.Writer); ok {
		return bufio.NewWriterSize(wd, size), nil
	} else {
		return nil, utils.Errorf("not support type: %v", reflect.TypeOf(i))
	}
}

func _newReadWriter(i, i2 interface{}) (*bufio.ReadWriter, error) {
	var (
		rd  *bufio.Reader
		wd  *bufio.Writer
		err error
	)

	rd, err = _newReader(i)
	if err != nil {
		return nil, err
	}
	wd, err = _newWriter(i2)
	if err != nil {
		return nil, err
	}

	return bufio.NewReadWriter(rd, wd), nil
}

func _newScanner(i interface{}) (*bufio.Scanner, error) {
	if rd, ok := i.(io.Reader); ok {
		return bufio.NewScanner(rd), nil
	} else {
		return nil, utils.Errorf("not support type: %v", reflect.TypeOf(i))
	}
}

var BufioExport = map[string]interface{}{
	"NewReader":     _newReader,
	"NewReaderSize": _newReaderSize,
	"NewWriter":     _newWriter,
	"NewWriterSize": _newWriterSize,
	"NewReadWriter": _newReadWriter,
	"NewScanner":    _newScanner,
}
