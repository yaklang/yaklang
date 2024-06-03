package base

type IUserProfile interface {
	UserProfile() ([]byte, error)
}
