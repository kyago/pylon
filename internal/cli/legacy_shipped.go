package cli

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"sort"
	"strings"
)

// legacy_shipped.txt records the sha256 of every built-in file content that releases
// 2026.3.1–2026.9.2 shipped. Those releases copied built-ins straight into the
// git-tracked .pylon/<kind>/ directories; from 2026.10.1 built-ins live in
// .pylon/builtin/, so this record is frozen and never needs new entries.
// Generated per tag with: git show <tag>:internal/cli/<rel> | shasum -a 256
//
//go:embed legacy_shipped.txt
var legacyShippedData string

// legacyShipped maps a built-in path (e.g. "agents/architect.md") to the set of
// content hashes pylon ever shipped for it.
var legacyShipped = parseLegacyShipped(legacyShippedData)

func parseLegacyShipped(data string) map[string]map[string]bool {
	table := map[string]map[string]bool{}
	for _, line := range strings.Split(data, "\n") {
		hash, rel, ok := strings.Cut(strings.TrimSpace(line), "  ")
		if !ok {
			continue
		}
		if table[rel] == nil {
			table[rel] = map[string]bool{}
		}
		table[rel][hash] = true
	}
	return table
}

// isLegacyShipped reports whether content is a copy of rel exactly as some release shipped it.
func isLegacyShipped(rel string, content []byte) bool {
	sum := sha256.Sum256(content)
	return legacyShipped[rel][hex.EncodeToString(sum[:])]
}

// retiredBuiltins lists, sorted, the previously shipped built-in paths that the
// current embeds no longer have.
func retiredBuiltins() ([]string, error) {
	files, err := embeddedBuiltinFiles()
	if err != nil {
		return nil, err
	}
	current := make(map[string]bool, len(files))
	for _, f := range files {
		current[f.rel] = true
	}
	var retired []string
	for rel := range legacyShipped {
		if !current[rel] {
			retired = append(retired, rel)
		}
	}
	sort.Strings(retired)
	return retired, nil
}
