// Package pgtypes holds the small PostgreSQL column types shared by GORM
// models.
//
// It replaces gorm.io/datatypes for JSONB columns: that module drags the MySQL
// driver stack into a PostgreSQL-only system, while a JSONB column only needs a
// value encoder and a scanner.
package pgtypes

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSON is a JSONB column value.
//
// It behaves like encoding/json.RawMessage but implements driver.Valuer and
// sql.Scanner, so it survives both the write path and the read path.
type JSON []byte

// NewJSON marshals v into a JSON column value.
func NewJSON(v any) (JSON, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	return JSON(raw), nil
}

// Value implements driver.Valuer. A nil value is written as SQL NULL so the
// column default applies.
func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}

	return []byte(j), nil
}

// Scan implements sql.Scanner.
func (j *JSON) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*j = nil
	case []byte:
		*j = append((*j)[:0], v...)
	case string:
		*j = append((*j)[:0], v...)
	default:
		return fmt.Errorf("pgtypes: cannot scan %T into JSON", src)
	}

	return nil
}

// MarshalJSON implements json.Marshaler so the column value can be returned
// verbatim inside API payloads.
func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}

	return j, nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *JSON) UnmarshalJSON(b []byte) error {
	*j = append((*j)[:0], b...)

	return nil
}

// String returns the raw JSON text.
func (j JSON) String() string { return string(j) }
