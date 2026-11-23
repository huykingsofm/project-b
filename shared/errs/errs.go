// Package errs holds what the project needs of errors besides the standard library. It imports
// nothing of the project.
package errs

import "errors"

// Join joins what registering many kinds returned, so that a game registers all and reports once.
func Join(errs ...error) error { return errors.Join(errs...) }
