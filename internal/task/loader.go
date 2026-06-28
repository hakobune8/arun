package task

// Loader wraps the Load function as a reusable struct for dependency
// injection.
type Loader struct{}

// NewLoader returns a new Loader.
func NewLoader() *Loader {
	return &Loader{}
}

// Load reads a Task from the YAML file at path by delegating to the package-
// level Load function.
func (l *Loader) Load(path string) (*Task, error) {
	return Load(path)
}
