package bottoken

import "testing"

func TestGenerate_HasPrefixAndExpectedLength(t *testing.T) {
	tok, err := Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	parsed, err := ParsePlaintext(tok)
	if err != nil {
		t.Fatalf("ParsePlaintext rejected freshly-generated token: %v", err)
	}

	if parsed != tok {
		t.Fatalf("ParsePlaintext mutated the token: got %q want %q", parsed, tok)
	}
}

func TestHashAndVerify(t *testing.T) {
	tok, err := Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	hashed := Hash(tok)
	if len(hashed) != 64 {
		t.Fatalf("Hash length = %d, want 64", len(hashed))
	}

	if !VerifyHash(tok, hashed) {
		t.Fatal("VerifyHash returned false for matching pair")
	}

	if VerifyHash(tok+"x", hashed) {
		t.Fatal("VerifyHash returned true for tampered plaintext")
	}
}

func TestParsePlaintext_RejectsBadInput(t *testing.T) {
	cases := []string{
		"",
		"bearer something",
		Prefix,
		Prefix + "tooshort",
		Prefix + "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ", // wrong charset
	}
	for _, c := range cases {
		if _, err := ParsePlaintext(c); err == nil {
			t.Errorf("ParsePlaintext(%q) = nil, want error", c)
		}
	}
}

func TestGenerateWebhookSecret_DeterministicLength(t *testing.T) {
	secret, err := GenerateWebhookSecret()
	if err != nil {
		t.Fatalf("GenerateWebhookSecret failed: %v", err)
	}

	if len(secret) != 64 {
		t.Fatalf("secret length = %d, want 64 hex chars", len(secret))
	}
}
