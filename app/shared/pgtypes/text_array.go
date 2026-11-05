package pgtypes

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// TextArray is a PostgreSQL text[] column value. It replaces pq.StringArray so
// the PostgreSQL driver stays single (pgx via GORM).
type TextArray []string

// textArrayEscaper doubles the characters PostgreSQL array syntax treats
// specially inside a quoted element.
var textArrayEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

// Value implements driver.Valuer. A nil slice is written as SQL NULL so the
// column default applies; every element is quoted, which is always legal.
func (a TextArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}

	var b strings.Builder
	b.WriteByte('{')
	for i, elem := range a {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		b.WriteString(textArrayEscaper.Replace(elem))
		b.WriteByte('"')
	}
	b.WriteByte('}')

	return b.String(), nil
}

// Scan implements sql.Scanner.
func (a *TextArray) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*a = nil
		return nil
	case []byte:
		return a.parse(string(v))
	case string:
		return a.parse(v)
	default:
		return fmt.Errorf("pgtypes: cannot scan %T into TextArray", src)
	}
}

func (a *TextArray) parse(src string) error {
	if len(src) < 2 || src[0] != '{' || src[len(src)-1] != '}' {
		return fmt.Errorf("pgtypes: unable to parse text array %q", src)
	}

	body := src[1 : len(src)-1]
	if strings.HasPrefix(body, "{") {
		return fmt.Errorf("pgtypes: multidimensional text arrays are not supported")
	}

	out := make(TextArray, 0, strings.Count(body, ",")+1)
	for i := 0; i < len(body); {
		var elem string
		if body[i] == '"' {
			var b strings.Builder
			i++
			for {
				if i >= len(body) {
					return fmt.Errorf("pgtypes: unable to parse text array %q", src)
				}
				c := body[i]
				if c == '"' {
					i++
					break
				}
				if c == '\\' {
					i++
					if i >= len(body) {
						return fmt.Errorf("pgtypes: unable to parse text array %q", src)
					}
					c = body[i]
				}
				b.WriteByte(c)
				i++
			}
			elem = b.String()
		} else {
			start := i
			for i < len(body) && body[i] != ',' {
				i++
			}
			switch raw := body[start:i]; raw {
			case "":
				return fmt.Errorf("pgtypes: unable to parse text array %q", src)
			case "NULL":
				return fmt.Errorf("pgtypes: text array element is NULL")
			default:
				elem = raw
			}
		}

		if i < len(body) {
			if body[i] != ',' {
				return fmt.Errorf("pgtypes: unable to parse text array %q", src)
			}
			i++
		}
		out = append(out, elem)
	}

	*a = out

	return nil
}
