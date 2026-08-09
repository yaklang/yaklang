//go:build !windows

package ssagitworkdir

func isPlatformCapacityError(error) bool {
	return false
}
