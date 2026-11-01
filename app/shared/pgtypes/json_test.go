package pgtypes

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSONScanBytesAndString(t *testing.T) {
	var j JSON
	require.NoError(t, j.Scan([]byte(`{"a":1}`)))
	require.JSONEq(t, `{"a":1}`, j.String())

	require.NoError(t, j.Scan(`[1,2]`))
	require.JSONEq(t, `[1,2]`, j.String())

	require.NoError(t, j.Scan(nil))
	require.Empty(t, j)
}

func TestJSONScanRejectsUnknownSource(t *testing.T) {
	var j JSON
	require.Error(t, j.Scan(42))
}

func TestJSONValueNilWritesSQLNull(t *testing.T) {
	// A nil JSON must reach the database as NULL so the DDL default applies.
	v, err := JSON(nil).Value()
	require.NoError(t, err)
	require.Nil(t, v)
}

func TestJSONMarshalsVerbatim(t *testing.T) {
	raw, err := json.Marshal(JSON(`{"kind":"image"}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"kind":"image"}`, string(raw))

	raw, err = json.Marshal(JSON(nil))
	require.NoError(t, err)
	require.Equal(t, "null", string(raw))
}

func TestNewJSONRoundTrip(t *testing.T) {
	j, err := NewJSON(map[string]int{"width": 640})
	require.NoError(t, err)
	require.JSONEq(t, `{"width":640}`, j.String())
}
