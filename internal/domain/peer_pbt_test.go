package domain

import (
	"regexp"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Feature: peer-template-name, Property 1: Template variable resolution
// For any PeerNameTemplateData, each variable resolves to the correct field value.
// Validates: Requirements 1.3, 1.4, 4.1, 4.2, 4.3, 4.4, 4.5
func TestProperty1_TemplateVariableResolution(t *testing.T) {
	// generator for safe alphanumeric strings (no template metacharacters)
	safeStr := rapid.StringMatching(`[A-Za-z0-9@._\- ]{0,32}`)

	rapid.Check(t, func(t *rapid.T) {
		data := PeerNameTemplateData{
			Id:        rapid.StringMatching(`[A-Za-z0-9]{8}`).Draw(t, "Id"),
			Random:    rapid.StringMatching(`[A-Za-z0-9]{8}`).Draw(t, "Random"),
			Email:     safeStr.Draw(t, "Email"),
			Firstname: safeStr.Draw(t, "Firstname"),
			Lastname:  safeStr.Draw(t, "Lastname"),
			PeerName:  safeStr.Draw(t, "PeerName"),
		}

		cases := []struct {
			tmpl     string
			expected string
		}{
			{"{{.Id}}", data.Id},
			{"{{.Random}}", data.Random},
			{"{{.Email}}", data.Email},
			{"{{.Firstname}}", data.Firstname},
			{"{{.Lastname}}", data.Lastname},
			{"{{.PeerName}}", data.PeerName},
		}

		for _, tc := range cases {
			result, err := ApplyPeerNameTemplate(tc.tmpl, data)
			if err != nil {
				t.Fatalf("unexpected error for template %q: %v", tc.tmpl, err)
			}
			if result != tc.expected {
				t.Fatalf("template %q: got %q, want %q", tc.tmpl, result, tc.expected)
			}
		}
	})
}

// Feature: peer-template-name, Property 2: Random variable is 8-char alphanumeric
// Validates: Requirements 4.6
func TestProperty2_RandomVariable8CharAlphanumeric(t *testing.T) {
	alphanumeric := regexp.MustCompile(`^[A-Za-z0-9]{8}$`)

	// Test generateRandomString directly (same package)
	rapid.Check(t, func(t *rapid.T) {
		r := generateRandomString(8)
		if !alphanumeric.MatchString(r) {
			t.Fatalf("generateRandomString(8) returned %q, want 8 alphanumeric chars", r)
		}
	})

	// Also test via template resolution
	rapid.Check(t, func(t *rapid.T) {
		data := PeerNameTemplateData{
			Random: generateRandomString(8),
		}
		result, err := ApplyPeerNameTemplate("{{.Random}}", data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 8 {
			t.Fatalf("expected length 8, got %d (%q)", len(result), result)
		}
		if !alphanumeric.MatchString(result) {
			t.Fatalf("result %q contains non-alphanumeric characters", result)
		}
	})
}

// Feature: peer-template-name, Property 3: Template output non-empty for literal text
// Validates: Requirements 4.7
func TestProperty3_TemplateOutputNonEmptyForLiteralText(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a non-empty string that contains no {{ or }}
		literal := rapid.StringMatching(`[^{}]{1,64}`).Draw(t, "literal")
		// Ensure it has at least one non-whitespace character
		if strings.TrimSpace(literal) == "" {
			t.Skip()
		}
		// Ensure no {{ or }} sequences
		if strings.Contains(literal, "{{") || strings.Contains(literal, "}}") {
			t.Skip()
		}

		data := PeerNameTemplateData{}
		result, err := ApplyPeerNameTemplate(literal, data)
		if err != nil {
			t.Fatalf("unexpected error for literal %q: %v", literal, err)
		}
		if result == "" {
			t.Fatalf("expected non-empty result for literal template %q", literal)
		}
	})
}

// Feature: peer-template-name, Property 4: Template applied at peer creation
// Validates: Requirements 2.1, 2.5
func TestProperty4_TemplateAppliedAtPeerCreation(t *testing.T) {
	// Use templates that don't reference {{.Random}} so the result is deterministic.
	// We generate a user and a peer, call GenerateDisplayName, then verify the
	// DisplayName equals what ApplyPeerNameTemplate would produce with the same data.
	safeStr := rapid.StringMatching(`[A-Za-z0-9@._\- ]{0,32}`)

	rapid.Check(t, func(t *rapid.T) {
		email := safeStr.Draw(t, "Email")
		firstname := safeStr.Draw(t, "Firstname")
		lastname := safeStr.Draw(t, "Lastname")
		id := rapid.StringMatching(`[A-Za-z0-9]{8}`).Draw(t, "Id")

		// Pick a deterministic template (no {{.Random}})
		tmplChoice := rapid.IntRange(0, 4).Draw(t, "tmplChoice")
		templates := []string{
			"{{.Email}}",
			"{{.Firstname}}",
			"{{.Lastname}}",
			"{{.Id}}",
			"{{.PeerName}}",
		}
		tmpl := templates[tmplChoice]

		peer := &Peer{Identifier: PeerIdentifier(id)}
		user := &User{Email: email, Firstname: firstname, Lastname: lastname}

		peer.GenerateDisplayName("", tmpl, user)

		// Build the same data that GenerateDisplayName would have used
		expectedData := PeerNameTemplateData{
			Id:        id,
			Email:     email,
			Firstname: firstname,
			Lastname:  lastname,
			PeerName:  "Peer " + id,
			// Random is not used in these templates, so we can use any value
		}
		expected, err := ApplyPeerNameTemplate(tmpl, expectedData)
		if err != nil {
			t.Fatalf("unexpected error applying template %q: %v", tmpl, err)
		}

		if peer.DisplayName != expected {
			t.Fatalf("template %q: got DisplayName %q, want %q", tmpl, peer.DisplayName, expected)
		}
	})
}
