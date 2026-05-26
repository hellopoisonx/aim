package s3signer

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

type S3Signer struct {
	Endpoint       string
	PublicEndpoint string
	Region         string
	AccessKey      string
	SecretKey      string
	Bucket         string
}

func (s S3Signer) Presign(method, key string, ttl time.Duration, extraHeaders map[string]string) (string, map[string]string, error) {
	endpoint := s.PublicEndpoint
	if endpoint == "" {
		endpoint = s.Endpoint
	}
	return s.presignWithEndpoint(endpoint, method, key, ttl, extraHeaders)
}

func (s S3Signer) PresignInternal(method, key string, ttl time.Duration, extraHeaders map[string]string) (string, map[string]string, error) {
	endpoint := s.Endpoint
	if endpoint == "" {
		endpoint = s.PublicEndpoint
	}
	return s.presignWithEndpoint(endpoint, method, key, ttl, extraHeaders)
}

func (s S3Signer) presignWithEndpoint(endpoint, method, key string, ttl time.Duration, extraHeaders map[string]string) (string, map[string]string, error) {
	if endpoint == "" || s.Bucket == "" {
		return "", nil, fmt.Errorf("s3 endpoint and bucket are required")
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	u, err := url.Parse(strings.TrimRight(endpoint, "/") + "/" + s.Bucket + "/" + strings.TrimLeft(key, "/"))
	if err != nil {
		return "", nil, err
	}

	headers := map[string]string{"host": u.Host}
	for k, v := range extraHeaders {
		if v == "" {
			continue
		}
		headers[strings.ToLower(k)] = strings.TrimSpace(v)
	}

	// Local SeaweedFS S3 can run without credentials. In that mode the URL is an
	// unsigned object URL while AIM still gates its issuance.
	if s.AccessKey == "" || s.SecretKey == "" {
		delete(headers, "host")
		return u.String(), headers, nil
	}

	now := time.Now().UTC()
	date := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")
	region := s.Region
	if region == "" {
		region = "us-east-1"
	}
	credentialScope := date + "/" + region + "/s3/aws4_request"

	query := u.Query()
	query.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	query.Set("X-Amz-Credential", s.AccessKey+"/"+credentialScope)
	query.Set("X-Amz-Date", amzDate)
	query.Set("X-Amz-Expires", fmt.Sprintf("%d", int(ttl.Seconds())))
	signedHeaders := canonicalHeaderNames(headers)
	query.Set("X-Amz-SignedHeaders", signedHeaders)
	u.RawQuery = query.Encode()

	canonicalRequest := strings.Join([]string{
		method,
		escapePath(u.EscapedPath()),
		canonicalQuery(u.Query()),
		canonicalHeaders(headers),
		signedHeaders,
		"UNSIGNED-PAYLOAD",
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		hexSHA256(canonicalRequest),
	}, "\n")
	signature := hex.EncodeToString(hmacSHA256(signingKey(s.SecretKey, date, region), stringToSign))
	query = u.Query()
	query.Set("X-Amz-Signature", signature)
	u.RawQuery = query.Encode()

	delete(headers, "host")
	return u.String(), headers, nil
}

func canonicalHeaderNames(headers map[string]string) string {
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, strings.ToLower(k))
	}
	sort.Strings(keys)
	return strings.Join(keys, ";")
}

func canonicalHeaders(headers map[string]string) string {
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, strings.ToLower(k))
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte(':')
		b.WriteString(strings.Join(strings.Fields(headers[k]), " "))
		b.WriteByte('\n')
	}
	return b.String()
}

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0)
	for _, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.ReplaceAll(strings.Join(parts, "&"), "+", "%20")
}

func escapePath(p string) string {
	if p == "" {
		return "/"
	}
	return p
}

func hexSHA256(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(data))
	return h.Sum(nil)
}

func signingKey(secret, date, region string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	return hmacSHA256(kService, "aws4_request")
}
