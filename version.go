package main

import (
	"strings"

	goversion "github.com/hashicorp/go-version"
)

// ---- Versionsvergleich ----
func normalizeVersion(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	s = strings.ReplaceAll(s, "_", ".")
	s = strings.ReplaceAll(s, ",", ".")
	return s
}

func isBehind(local, upstream string) bool {
	vl, el := goversion.NewVersion(normalizeVersion(local))
	vu, eu := goversion.NewVersion(normalizeVersion(upstream))

	if el == nil && eu == nil {
		return vl.LessThan(vu)
	}

	// Fallback: sehr einfach (MVP)
	return local < upstream
}
