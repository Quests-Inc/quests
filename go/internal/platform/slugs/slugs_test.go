package slugs

import (
	"strings"
	"testing"
)

func TestValid(t *testing.T) {
	for _, value := range []string{"main", "project-12", strings.Repeat("a", 63)} {
		if !Valid(value) {
			t.Errorf("rejected %q", value)
		}
	}
	for _, value := range []string{"", "Upper", "two--hyphens", "-leading", "trailing-", "space here", "../path", "кириллица", strings.Repeat("a", 64)} {
		if Valid(value) {
			t.Errorf("accepted %q", value)
		}
	}
}
