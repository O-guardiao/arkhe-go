package seahorse

import (
	"regexp"
	"strings"
)

// phraseRegex matches complete quoted phrases like "exact phrase".
// Compiled once at package level to avoid per-call overhead.
var phraseRegex = regexp.MustCompile(`"([^"]+)"`)

// SanitizeFTS5Query escapes user input for safe use in an FTS5 MATCH expression.
//
// FTS5 treats certain characters as operators:
//   - `-` (NOT), `+` (required), `*` (prefix), `^` (initial token)
//   - `OR`, `AND`, `NOT`, `NEAR` (boolean/proximity operators)
//   - `:` (column filter — e.g. `agent:foo` means "search column agent")
//   - `"` (phrase query), `(` `)` (grouping)
//
// Strategy: wrap each whitespace-delimited token in double quotes so FTS5
// treats it as a literal phrase token. User-quoted phrases ("...") are
// preserved as-is. Internal double quotes are stripped. Empty tokens are
// dropped. Tokens are joined with spaces (implicit AND).
//
// Returns empty string for blank input so callers can skip the MATCH query.
//
// Examples:
//
//	"sub-agent restrict"  →  `"sub-agent" "restrict"`
//	"lcm_expand OR crash" →  `"lcm_expand" "OR" "crash"`
//	`hello "world"`       →  `"hello" "world"`
func SanitizeFTS5Query(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	// Preserve user-quoted phrases: extract "..." groups first, then tokenize the rest.
	var parts []string
	lastIndex := 0

	for _, loc := range phraseRegex.FindAllStringIndex(raw, -1) {
		// Process unquoted text before this phrase
		before := raw[lastIndex:loc[0]]
		for _, t := range strings.Fields(before) {
			t = strings.ReplaceAll(t, `"`, "")
			if t != "" {
				for _, sub := range strings.Split(t, ":") {
					sub = strings.TrimSpace(sub)
					if sub != "" {
						parts = append(parts, `"`+sub+`"`)
					}
				}
			}
		}
		// Preserve the phrase as-is (strip internal quotes and colons for safety)
		phrase := strings.TrimSpace(strings.ReplaceAll(raw[loc[0]+1:loc[1]-1], `"`, ""))
		if phrase != "" {
			// Colons inside quoted phrases also trigger column filter; split them.
			for _, sub := range strings.Split(phrase, ":") {
				sub = strings.TrimSpace(sub)
				if sub != "" {
					parts = append(parts, `"`+sub+`"`)
				}
			}
		}
		lastIndex = loc[1]
	}

	// Process unquoted text after last phrase
	for _, t := range strings.Fields(raw[lastIndex:]) {
		t = strings.ReplaceAll(t, `"`, "")
		if t != "" {
			// Colon is a column filter in FTS5 even inside quotes;
			// split on colon so each part is searched as a literal token.
			for _, sub := range strings.Split(t, ":") {
				sub = strings.TrimSpace(sub)
				if sub != "" {
					parts = append(parts, `"`+sub+`"`)
				}
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}
