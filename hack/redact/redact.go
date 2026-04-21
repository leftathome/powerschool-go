// Package main implements a CLI that redacts real student data out of
// raw PowerSchool HTML/JSON captures before those captures land in
// committed test fixtures.
//
// The redactor is intentionally dumb: it does byte-level substring
// substitution driven by a gitignored config file
// (hack/redact.config.json) that maps real values (names, UIDs, emails,
// hosts) to stable placeholders. Substitutions are applied
// longest-key-first so that "Student Alpha Smith" is replaced before
// "Student Alpha", and the tool refuses to run if any placeholder
// contains a find key (which would break idempotence).
//
// Invoke via: go run ./hack/redact -in <capture> -out <fixture> [-config <path>]
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Config is the JSON schema of hack/redact.config.json.
//
// The file itself is gitignored — each contributor populates it from
// their own captures. The example committed alongside the source
// (hack/redact.config.example.json) documents the expected shape.
type Config struct {
	// Replacements maps real-world strings to stable placeholders.
	// Keys are looked up as raw substrings in the input, so a UID
	// embedded in a URL or an HTML attribute will match the same
	// way a name embedded in text does.
	Replacements map[string]string `json:"replacements"`
}

// LoadConfig reads and parses a redaction config from path.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("redact: read config %q: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("redact: parse config %q: %w", path, err)
	}
	return &c, nil
}

// Redact returns input with every configured replacement applied.
//
// Guarantees:
//   - Longest-key-first: overlapping prefixes ("Student" vs
//     "Student Alpha") resolve in favor of the longer key.
//   - Deterministic: equal keys are applied in sorted order (Go maps
//     have randomized iteration).
//   - Idempotent: Redact(Redact(x)) == Redact(x), verified by
//     validating that no replacement value contains any find key as
//     a substring before the first pass.
func Redact(input string, c *Config) (string, error) {
	if c == nil || len(c.Replacements) == 0 {
		return input, nil
	}

	keys := make([]string, 0, len(c.Replacements))
	for k, v := range c.Replacements {
		if k == "" {
			return "", fmt.Errorf("redact: empty find key in config")
		}
		if v == "" {
			return "", fmt.Errorf("redact: empty replacement for key %q", k)
		}
		keys = append(keys, k)
	}

	// Longest first, ties broken alphabetically for determinism.
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})

	// Idempotence guard: if any placeholder contains a find key, a
	// second pass would re-substitute inside a placeholder and the
	// tool would no longer be idempotent. Refuse up front.
	for _, k := range keys {
		for _, other := range keys {
			if strings.Contains(c.Replacements[other], k) {
				return "", fmt.Errorf(
					"redact: replacement for %q contains find key %q — "+
						"would break idempotence",
					other, k,
				)
			}
		}
	}

	out := input
	for _, k := range keys {
		out = strings.ReplaceAll(out, k, c.Replacements[k])
	}
	return out, nil
}
