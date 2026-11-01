package s3signer

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newSigner() S3Signer {
	return S3Signer{
		Endpoint:       "http://seaweed-s3:8333",
		PublicEndpoint: "http://localhost:8333",
		Region:         "us-east-1",
		AccessKey:      "access",
		SecretKey:      "secret",
		Bucket:         "aim-attachments",
	}
}

func TestPresignUsesPublicEndpoint(t *testing.T) {
	u, headers, err := newSigner().Presign("GET", "attachments/a.png", time.Minute, nil)
	require.NoError(t, err)

	parsed, err := url.Parse(u)
	require.NoError(t, err)
	require.Equal(t, "localhost:8333", parsed.Host)
	require.Equal(t, "/aim-attachments/attachments/a.png", parsed.Path)
	require.NotEmpty(t, parsed.Query().Get("X-Amz-Signature"))
	// The host header is consumed by signing, not returned to the client.
	require.NotContains(t, headers, "host")
}

func TestPresignInternalUsesClusterEndpoint(t *testing.T) {
	u, _, err := newSigner().PresignInternal("HEAD", "attachments/a.png", time.Minute, nil)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(u, "http://seaweed-s3:8333/aim-attachments/"), "got %s", u)
}

func TestPresignSignatureDependsOnMethodAndKey(t *testing.T) {
	signer := newSigner()

	getURL, _, err := signer.Presign("GET", "attachments/a.png", time.Minute, nil)
	require.NoError(t, err)
	putURL, _, err := signer.Presign("PUT", "attachments/a.png", time.Minute, nil)
	require.NoError(t, err)
	otherKeyURL, _, err := signer.Presign("GET", "attachments/b.png", time.Minute, nil)
	require.NoError(t, err)

	require.NotEqual(t, signatureOf(getURL), signatureOf(putURL), "method must be part of the signature")
	require.NotEqual(t, signatureOf(getURL), signatureOf(otherKeyURL), "object key must be part of the signature")
}

func TestPresignIncludesExtraHeadersInSignedHeaders(t *testing.T) {
	_, headers, err := newSigner().Presign("PUT", "attachments/a.png", time.Minute, map[string]string{
		"Content-Type": "image/png",
	})
	require.NoError(t, err)
	require.Equal(t, "image/png", headers["content-type"])

	u, _, err := newSigner().Presign("PUT", "attachments/a.png", time.Minute, map[string]string{
		"Content-Type": "image/png",
	})
	require.NoError(t, err)

	parsed, err := url.Parse(u)
	require.NoError(t, err)
	require.Equal(t, "content-type;host", parsed.Query().Get("X-Amz-SignedHeaders"))
}

func TestPresignEmptyCredentialsYieldsUnsignedURL(t *testing.T) {
	signer := S3Signer{Endpoint: "http://localhost:8333", Bucket: "aim-attachments"}

	u, headers, err := signer.Presign("GET", "attachments/a.png", time.Minute, nil)
	require.NoError(t, err)
	require.NotContains(t, u, "X-Amz-Signature")
	require.Empty(t, headers["host"], "the host header is dropped for unsigned URLs")
}

func TestPresignRequiresEndpointAndBucket(t *testing.T) {
	_, _, err := S3Signer{}.Presign("GET", "k", time.Minute, nil)
	require.Error(t, err)

	_, _, err = S3Signer{Endpoint: "http://localhost:8333"}.Presign("GET", "k", time.Minute, nil)
	require.Error(t, err)
}

func TestPresignDefaultsTTL(t *testing.T) {
	u, _, err := newSigner().Presign("GET", "attachments/a.png", 0, nil)
	require.NoError(t, err)

	parsed, err := url.Parse(u)
	require.NoError(t, err)
	require.Equal(t, "900", parsed.Query().Get("X-Amz-Expires"))
}

func signatureOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	return parsed.Query().Get("X-Amz-Signature")
}
