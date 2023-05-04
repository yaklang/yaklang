package yakit

import "sync"

var __initializingDatabase []func() error
var __mutexForInit = new(sync.Mutex)

func RegisterPostInitDatabaseFunction(f func() error) {
	__mutexForInit.Lock()
	defer __mutexForInit.Unlock()
	__initializingDatabase = append(__initializingDatabase, f)
}

func CallPostInitDatabase() error {
	for _, f := range __initializingDatabase {
		err := f()
		if err != nil {
			return err
		}
	}
	return nil
}
