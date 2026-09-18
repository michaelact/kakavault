// Package classify assigns a classification (e.g. "internal",
// "third-party") to a Secret key name, using an ordered list of regex
// rules plus an exact-match override map that always wins.
package classify

import (
	"fmt"
	"regexp"

	"github.com/michaelact/kakavault/internal/config"
)

type compiledRule struct {
	pattern        *regexp.Regexp
	classification string
}

// Classifier holds pre-compiled rules for repeated Classify calls.
type Classifier struct {
	rules     []compiledRule
	overrides map[string]string
}

// New compiles cfg's rules once so Classify is cheap to call per key.
func New(cfg config.Classification) (*Classifier, error) {
	rules := make([]compiledRule, 0, len(cfg.Rules))
	for i, r := range cfg.Rules {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("rule[%d] pattern %q: %w", i, r.Pattern, err)
		}
		rules = append(rules, compiledRule{pattern: re, classification: r.Classification})
	}

	return &Classifier{rules: rules, overrides: cfg.Overrides}, nil
}

// Classify returns key's classification. An override always wins; failing
// that, the first matching rule (in config order) wins; if nothing
// matches, it returns "unclassified" rather than guessing.
func (c *Classifier) Classify(key string) string {
	if override, ok := c.overrides[key]; ok {
		return override
	}
	for _, r := range c.rules {
		if r.pattern.MatchString(key) {
			return r.classification
		}
	}
	return "unclassified"
}
