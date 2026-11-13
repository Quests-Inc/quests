// Package slugs validates stable URL names shared by workspaces and repositories.
package slugs

import "regexp"

var format = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func Valid(value string) bool { return len(value) <= 63 && format.MatchString(value) }
