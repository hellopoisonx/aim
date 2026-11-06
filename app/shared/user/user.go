// Package user holds the closed sets stored in the user_info table.
package user

import "fmt"

// Type is the closed set of user_info.user_type values.
type Type string

const (
	TypeHuman Type = "human"
	TypeBot   Type = "bot"
)

// ParseType validates a stored user_type instead of guessing a default.
func ParseType(s string) (Type, error) {
	switch t := Type(s); t {
	case TypeHuman, TypeBot:
		return t, nil
	}

	return "", fmt.Errorf("user: unknown user_type %q", s)
}

// Status is the closed set of user_info.status values.
type Status int16

const (
	StatusDisabled Status = 0
	StatusEnabled  Status = 1
)
