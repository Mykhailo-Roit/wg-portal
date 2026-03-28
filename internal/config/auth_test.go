package config

import "testing"

func TestSanitizeProviderDisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		displayName  string
		providerName string
		expected     string
	}{
		{
			name:         "fallback to provider name",
			displayName:  "   ",
			providerName: "oidc-main",
			expected:     "oidc-main",
		},
		{
			name:         "strip control characters",
			displayName:  "Login\twith\r\nGoogle\x00",
			providerName: "google",
			expected:     "Login with  Google",
		},
		{
			name:         "keep plain text",
			displayName:  "<b>Company Login</b>",
			providerName: "company",
			expected:     "<b>Company Login</b>",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := sanitizeProviderDisplayName(tt.displayName, tt.providerName); got != tt.expected {
				t.Fatalf("sanitizeProviderDisplayName() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAuthSanitize(t *testing.T) {
	t.Parallel()

	cfg := Auth{
		OpenIDConnect: []OpenIDConnectProvider{{
			ProviderName: "oidc-main",
			DisplayName:  "  Sign in with OIDC  ",
		}},
		OAuth: []OAuthProvider{{
			ProviderName: "legacy-oauth",
			DisplayName:  "",
		}},
	}

	warnings := cfg.Sanitize()

	if cfg.OpenIDConnect[0].DisplayName != "Sign in with OIDC" {
		t.Fatalf("unexpected OIDC display name: %q", cfg.OpenIDConnect[0].DisplayName)
	}
	if cfg.OAuth[0].DisplayName != "legacy-oauth" {
		t.Fatalf("unexpected OAuth display name fallback: %q", cfg.OAuth[0].DisplayName)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected one warning, got %d", len(warnings))
	}
}
