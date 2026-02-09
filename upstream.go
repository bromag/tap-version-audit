package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type kind int

const (
	kindUnknown kind = iota
	kindFormula
	kindCask
)

type tapEntry struct {
	name    string
	Version string
	Path    string
	Kind    kind
}

var upstreamOverrides = map[string]string{
	"gov-filter-repo":        "git-filter-repo",
	"gov-md2man":             "go-md2man",
	"gov-swift-package-list": "swift-package-list",
	"gov-shebang-probe":      "scriptisto",
}

var externalTapRawRB = map[string]string{
	// Formulae, die NICHT in homebrew/core sind, aber in bekannten Taps liegen:
	// Key = upstream formula name (ohne gov- Prefix, ohne @patch)
	// Value = raw URL zur .rb
	// sdkman-cli liegt im Tap sdkman/tap (Repo sdkman/homebrew-tap)
	"sdkman-cli": "https://raw.githubusercontent.com/sdkman/homebrew-tap/master/Formula/sdkman-cli.rb",
	// danger-js liegt im Tap danger/tap (Repo danger/homebrew-tap)
	"danger-js": "https://raw.githubusercontent.com/danger/homebrew-tap/master/danger-js.rb",
	// swift-package-list im Tap swift/tap (Repo swift/homebrew-tap)
	"swift-package-list": "https://raw.githubusercontent.com/FelixHerrmann/homebrew-tap/master/Formula/swift-package-list.rb",
}

// ---- API Response Struct ----
type formulaAPIResponse struct {
	Name     string `json:"name"`
	Versions struct {
		Stable string `json:"stable"`
	} `json:"versions"`
}

// ---- Mapping: private name -> upstream name ----
// Start simpel: gov-foo -> foo
func toUpstreamName(private string) string {
	s := strings.TrimPrefix(private, "gov-")

	// Overrides
	if v, ok := upstreamOverrides[private]; ok {
		return v
	}

	at := strings.LastIndex(s, "@")
	if at == -1 {
		return s
	}

	base := s[:at]
	suffix := s[at+1:] // z.b 13.0.4

	// behalte major oder major.minor
	if isMajorOrMajorMinor(suffix) {
		return s
	}
	return base
}

func isMajorOrMajorMinor(x string) bool {
	parts := strings.Split(x, ".")
	if len(parts) == 1 {
		return allDigits(parts[0])
	}
	if len(parts) == 2 {
		return allDigits(parts[0]) && allDigits(parts[1])
	}
	return false
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ---- Upstream stable Version via API holen ----
func fetchUpstreamStable(client *http.Client, formula string) (stable string, ok bool, err error) {
	url := "https://formulae.brew.sh/api/formula/" + formula + ".json"

	resp, err := client.Get(url)
	if err != nil {
		return "", false, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			// optional: log.Printf("close resp body: %v", cerr)
		}
	}()

	if resp.StatusCode == http.StatusNotFound {
		rawURL, ok2 := externalTapRawRB[formula]
		if !ok2 {
			return "", false, nil
		}

		r2, err := client.Get(rawURL)
		if err != nil {
			return "", false, err
		}
		defer func() {
			if cerr := r2.Body.Close(); cerr != nil {
				// optional: log.Printf("close resp body: %v", cerr)
			}
		}()

		if r2.StatusCode == http.StatusNotFound {
			return "", false, nil
		}
		if r2.StatusCode < 200 || r2.StatusCode >= 300 {
			return "", false, fmt.Errorf("tap raw http status %d", r2.StatusCode)
		}

		body, err := io.ReadAll(r2.Body)
		if err != nil {
			return "", false, err
		}

		v := strings.TrimSpace(extractVersion(string(body), formula))
		if v == "" {
			return "", false, nil
		}
		return v, true, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false, fmt.Errorf("upstream http status %d", resp.StatusCode)
	}

	var data formulaAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", false, err
	}

	stable = strings.TrimSpace(data.Versions.Stable)
	if stable == "" {
		return "", false, nil
	}
	return stable, true, nil
}
