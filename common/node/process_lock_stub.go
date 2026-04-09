//go:build !unix

package node

type nodeInstanceLock struct{}

func acquireNodeInstanceLock(_ string) (*nodeInstanceLock, error) {
	return &nodeInstanceLock{}, nil
}

func (l *nodeInstanceLock) Release() error {
	return nil
}
