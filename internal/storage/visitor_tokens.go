package storage

import "github.com/google/uuid"

func newVisitorToken(value string) string {
	if value != "" {
		return value
	}
	return uuid.NewString()
}
