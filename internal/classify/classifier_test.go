package classify

import (
	"testing"

	"github.com/michaelact/kakavault/internal/config"
)

func testConfig() config.Classification {
	return config.Classification{
		Rules: []config.ClassificationRule{
			{Pattern: "(API_KEY|PASSWORD|_DB_URL)$", Classification: "third-party"},
			{Pattern: ".*", Classification: "internal"},
		},
		Overrides: map[string]string{
			"ADMIN_API_KEY": "internal",
		},
	}
}

func TestClassify_RuleMatch(t *testing.T) {
	c, err := New(testConfig())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cases := map[string]string{
		"OPENAI_API_KEY":                  "third-party",
		"DATABASE_PASSWORD":               "third-party",
		"BACKEND_SSO_DATABASE_URL_DB_URL": "third-party",
		"ENCRYPTION_KEY":                  "internal",
		"JWT_SECRET":                      "internal",
	}

	for key, want := range cases {
		if got := c.Classify(key); got != want {
			t.Errorf("Classify(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestClassify_OverrideWinsOverRule(t *testing.T) {
	c, err := New(testConfig())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// ADMIN_API_KEY matches the third-party rule (ends in API_KEY) but has
	// an explicit override to internal — override must win.
	if got := c.Classify("ADMIN_API_KEY"); got != "internal" {
		t.Errorf("Classify(ADMIN_API_KEY) = %q, want %q (override should win over rule)", got, "internal")
	}
}

func TestClassify_FirstRuleWins(t *testing.T) {
	cfg := config.Classification{
		Rules: []config.ClassificationRule{
			{Pattern: "^FOO", Classification: "first"},
			{Pattern: "BAR$", Classification: "second"},
		},
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// FOOBAR matches both rules; the first one in the list must win.
	if got := c.Classify("FOOBAR"); got != "first" {
		t.Errorf("Classify(FOOBAR) = %q, want %q (first matching rule should win)", got, "first")
	}
}

func TestClassify_NoRuleMatches(t *testing.T) {
	cfg := config.Classification{
		Rules: []config.ClassificationRule{
			{Pattern: "^ONLY_THIS$", Classification: "special"},
		},
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// No rule matches and there's no catch-all — Classify should return
	// the sentinel "unclassified" rather than panicking or guessing.
	if got := c.Classify("SOMETHING_ELSE"); got != "unclassified" {
		t.Errorf("Classify(SOMETHING_ELSE) = %q, want %q", got, "unclassified")
	}
}

func TestNew_InvalidPattern(t *testing.T) {
	cfg := config.Classification{
		Rules: []config.ClassificationRule{
			{Pattern: "(unclosed", Classification: "internal"},
		},
	}
	if _, err := New(cfg); err == nil {
		t.Fatal("New() error = nil, want an error for an invalid regex")
	}
}
