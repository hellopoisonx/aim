package attachment

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidKindIncludesGenericFile(t *testing.T) {
	require.True(t, ValidKind(KindImage))
	require.True(t, ValidKind(KindVideo))
	require.True(t, ValidKind(KindAudio))
	require.True(t, ValidKind(KindFile))
	require.False(t, ValidKind("document"))
}

func TestRequiresDataParsing(t *testing.T) {
	require.True(t, RequiresDataParsing(KindImage))
	require.True(t, RequiresDataParsing(KindVideo))
	require.True(t, RequiresDataParsing(KindAudio))
	require.False(t, RequiresDataParsing(KindFile))
	require.False(t, RequiresDataParsing("unknown"))
}

func TestFileContentValidates(t *testing.T) {
	content := Content{
		Schema:      ContentSchemaV1,
		FileID:      "file-id",
		Kind:        KindFile,
		Original:    OriginalObject{Name: "report.pdf", Mime: "application/pdf", Size: 1234},
		ParseStatus: ParseStatusReady,
	}

	_, err := content.Marshal()
	require.NoError(t, err)
}
