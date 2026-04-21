package errs

import (
	"errors"
	"strings"
)

// JoinProblems combines multiple error messages into a single error,
// removing duplicates and empty strings. Returns nil if nothing remains.
func JoinProblems(problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(problems))
	unique := make([]string, 0, len(problems))
	for _, p := range problems {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		unique = append(unique, p)
	}
	if len(unique) == 0 {
		return nil
	}
	return errors.New(strings.Join(unique, "; "))
}
