package idutil

import (
	"strconv"
	"strings"
)

// Parse parses a string ID to int64.
func Parse(id string) (int64, error) {
	return strconv.ParseInt(id, 10, 64)
}

// ParseSlice parses a slice of string IDs to int64 slice.
func ParseSlice(ids []string) ([]int64, error) {
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		v, err := Parse(id)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}

// Format formats an int64 ID to string.
func Format(id int64) string {
	return strconv.FormatInt(id, 10)
}

// FormatSlice formats a slice of int64 IDs to string slice.
func FormatSlice(ids []int64) []string {
	result := make([]string, len(ids))
	for i, id := range ids {
		result[i] = Format(id)
	}
	return result
}

// Join joins int64 IDs with comma.
func Join(ids []int64) string {
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = Format(id)
	}
	return strings.Join(strs, ",")
}
