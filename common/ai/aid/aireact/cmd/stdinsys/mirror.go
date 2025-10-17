package stdinsys

import (
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"sync"
)

type Mirror struct {
	w io.WriteCloser
	r io.Reader
}

func (m *Mirror) Write(r []byte) (int, error) {
	return m.w.Write(r)
}

func (m *Mirror) Read(readBuf []byte) (int, error) {
	return m.r.Read(readBuf)
}

func (m *Mirror) Close() error {
	return m.w.Close()
}

func newMirror() *Mirror {
	r, w := utils.NewPipe()
	return &Mirror{w, r}
}

type dynamicMultiWriter struct {
	sync.RWMutex

	ws map[string]io.Writer
}

func newDynamicMultiWriter() *dynamicMultiWriter {
	return &dynamicMultiWriter{
		ws: make(map[string]io.Writer),
	}
}

func (m *dynamicMultiWriter) AddWriter(name string, w io.Writer) {
	m.Lock()
	defer m.Unlock()
	m.ws[name] = w
}

func (m *dynamicMultiWriter) RemoveWriter(name string) {
	m.Lock()
	defer m.Unlock()
	if _, ok := m.ws[name]; ok {
		delete(m.ws, name)
	}
}

func (m *dynamicMultiWriter) Write(r []byte) (int, error) {
	m.RLock()
	defer m.RUnlock()

	for name, w := range m.ws {
		_ = name
		_, err := w.Write(r)
		if err != nil {
			continue
		}
	}
	return len(r), nil
}
