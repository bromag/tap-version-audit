package main

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// ---- Regex für Version / URL in Formula Ruby Files ----
var reVersion = regexp.MustCompile(`(?m)^\s*version(?:\s*\(\s*)?\s*["']([^"']+)["']`)
var reURL = regexp.MustCompile(`(?m)^\s*url\s+["']([^"']+)["']`)

// ---- Local Tap: Versions aus Formula Ruby Files laden ----
func loadFormulaVersions(repoPath string) (map[string]string, error) {
	formulaDir := filepath.Join(repoPath, "Formula")
	out := map[string]string{}

	err := filepath.WalkDir(formulaDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".rb") {
			return nil
		}
		name := strings.TrimSuffix(d.Name(), ".rb")

		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}

		v := extractVersion(string(b), name)
		if v != "" {
			out[name] = v
		}
		return nil
	})

	return out, err
}

// ---- Version aus Ruby Content extrahieren: version -> url fallback ----
func extractVersion(content, pkgName string) string {
	if m := reVersion.FindStringSubmatch(content); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	if m := reURL.FindStringSubmatch(content); len(m) == 2 {
		return inferVersionFromURL(m[1], pkgName)
	}
	return ""
}

// ---- URL -> Version ableiten ----
func stripExt(s string) string {
	exts := []string{
		".tar.gz", ".tar.bz2", ".tar.xz", ".tgz",
		".zip", ".gz", ".bz2", ".xz",
		".tar",
	}
	for _, e := range exts {
		if strings.HasSuffix(s, e) {
			return strings.TrimSuffix(s, e)
		}
	}
	if i := strings.LastIndex(s, "."); i > 0 {
		return s[:i]
	}
	return s
}

func inferVersionFromURL(u, pkgName string) string {
	base := stripExt(path.Base(strings.TrimSpace(u)))

	candidates := []string{
		base,
		strings.TrimPrefix(base, pkgName+"-"),
		strings.TrimPrefix(base, pkgName+"_"),
		strings.TrimPrefix(base, pkgName+"v"),
	}

	reVerToken := regexp.MustCompile(`v?(\d+(?:\.\d+)+[A-Za-z0-9._-]*)`)

	for _, c := range candidates {
		if m := reVerToken.FindStringSubmatch(c); len(m) == 2 {
			return strings.TrimPrefix(m[1], "v")
		}
	}

	if m := reVerToken.FindStringSubmatch(u); len(m) == 2 {
		return strings.TrimPrefix(m[1], "v")
	}

	return ""
}
