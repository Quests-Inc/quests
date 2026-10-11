package kernel

import (
	"net/http"
	"strings"
)

// StatusFor corrects the status of a failure that is really the caller's to fix, wherever it surfaced: a member
// writing a command is refused (403), a command without a runner is unprocessable (422). Other statuses pass through.
func StatusFor(status int, detail string) int {
	if status >= 500 {
		switch {
		case strings.Contains(detail, ErrCommandOwner.Error()):
			return http.StatusForbidden
		case strings.Contains(detail, ErrNoRunner.Error()):
			return http.StatusUnprocessableEntity
		}
	}
	return status
}
