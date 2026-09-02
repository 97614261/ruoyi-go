package service

// Password limits are measured in Unicode characters, matching validation
// and preventing non-ASCII passwords from being treated differently by each entry point.
const (
	passwordMinLength = 5
	passwordMaxLength = 20
)

func validPasswordLength(password string) bool {
	length := len([]rune(password))
	return length >= passwordMinLength && length <= passwordMaxLength
}
