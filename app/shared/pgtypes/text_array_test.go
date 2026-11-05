package pgtypes

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Expectations were produced by pq.StringArray before lib/pq was dropped.
func TestTextArrayValue(t *testing.T) {
	cases := []struct {
		name  string
		array TextArray
		want  any
	}{
		{"nil is SQL NULL", nil, nil},
		{"empty array", TextArray{}, "{}"},
		{"single element", TextArray{"message.created"}, `{"message.created"}`},
		{"element with comma", TextArray{"a,b"}, `{"a,b"}`},
		{"element with double quote", TextArray{`he said "hi"`}, `{"he said \"hi\""}`},
		{"element with backslash", TextArray{`a\b`}, `{"a\\b"}`},
		{"quoted NULL stays text", TextArray{"NULL"}, `{"NULL"}`},
		{"empty element", TextArray{"", "x"}, `{"","x"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.array.Value()
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestTextArrayScan(t *testing.T) {
	cases := []struct {
		name string
		src  any
		want TextArray
	}{
		{"SQL NULL", nil, nil},
		{"empty array", []byte(`{}`), TextArray{}},
		{"single element", []byte(`{"message.created"}`), TextArray{"message.created"}},
		{"element with comma", []byte(`{"a,b"}`), TextArray{"a,b"}},
		{"element with double quote", []byte(`{"he said \"hi\""}`), TextArray{`he said "hi"`}},
		{"element with backslash", []byte(`{"a\\b"}`), TextArray{`a\b`}},
		{"quoted NULL stays text", []byte(`{"NULL"}`), TextArray{"NULL"}},
		{"empty element", []byte(`{"","x"}`), TextArray{"", "x"}},
		{"unquoted elements", []byte(`{a,b}`), TextArray{"a", "b"}},
		{"string source", `{"x"}`, TextArray{"x"}},
		// Verbatim output of a real backend: select array['a,b','he said "hi"','back\slash','NULL','','plain']::text[].
		{"postgres backend output", []byte(`{"a,b","he said \"hi\"","back\\slash","NULL","",plain}`), TextArray{"a,b", `he said "hi"`, `back\slash`, "NULL", "", "plain"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got TextArray
			require.NoError(t, got.Scan(tc.src))
			require.Equal(t, tc.want, got)
		})
	}
}

func TestTextArrayScanRejects(t *testing.T) {
	cases := []struct {
		name string
		src  any
	}{
		{"unquoted NULL element", []byte(`{NULL}`)},
		{"multidimensional", []byte(`{{a},{b}}`)},
		{"missing closing brace", []byte(`{a`)},
		{"unterminated quote", []byte(`{"a`)},
		{"wrong source type", 42},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got TextArray
			require.Error(t, got.Scan(tc.src))
		})
	}
}

func TestTextArrayRoundTrip(t *testing.T) {
	in := TextArray{"a", "b,c", `d"e`, `f\g`, "NULL", ""}
	v, err := in.Value()
	require.NoError(t, err)

	var out TextArray
	require.NoError(t, out.Scan(v))
	require.Equal(t, in, out)
}
