package sandbox

// Policy defines sandbox write restrictions and file size limits.
type Policy struct {
	DenyWritePatterns []string
	MaxFileSize       int64
}

// NewPolicy returns a Policy with default deny patterns for secret files and
// a 1 MB max file size.
func NewPolicy() *Policy {
	return &Policy{
		DenyWritePatterns: []string{
			".env", ".env.*",
			"*.pem", "id_rsa", "id_ed25519",
		},
		MaxFileSize: 1024 * 1024,
	}
}
