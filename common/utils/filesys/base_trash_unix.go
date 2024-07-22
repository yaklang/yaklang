//go:build !windows

package filesys

// fallback to Delete on unix
func (f *LocalFs) Throw(filenames ...string) error {
	for _, filename := range filenames {
		if err := f.Delete(filename); err != nil {
			return err
		}
	}
	return nil
}
