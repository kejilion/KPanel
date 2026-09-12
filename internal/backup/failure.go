package backup

import "errors"

type Failure struct {
	Code string
	Err  error
}

func (e *Failure) Error() string { return e.Code }
func (e *Failure) Unwrap() error { return e.Err }
func FailureCode(err error) string {
	var failure *Failure
	if errors.As(err, &failure) {
		switch failure.Code {
		case "rolled_back", "recovery_required", "cleanup_pending", "partially_restored", "host_busy":
			return failure.Code
		}
	}
	return "operation_failed"
}
