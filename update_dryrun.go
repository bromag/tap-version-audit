package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

// updateOne holt das komplette Upstream-Ruby-File (.rb), passt es minimal an dein Gov-Naming an
// (aktuell nur die class-Zeile), zeigt eine Vorschau und schreibt es optional (apply=true)
// in dein lokales Mirror-Repo (.cache/private-tap/...).
//
// - client: wiederverwendeter HTTP Client (Timeout etc.)
// - privateName: z.B. "gov-abseil"
// - entry: enthält lokale Version & vor allem den Ziel-Pfad entry.Path
// - apply: false = nur anzeigen (Dry-run), true = Datei überschreiben
func updateOne(client *http.Client, privateName string, entry localFormula, apply bool) error {
	// 1) Private Name -> Upstream Name mappen (z.B. gov-abseil -> abseil)
	upName := toUpstreamName(privateName)

	// 2) Komplettes Upstream .rb holen (nicht nur Version!)
	//    rb = vollständiger Ruby-Text
	//    srcURL = woher es genau geladen wurde
	rb, srcURL, err := fetchUpStreamRB(client, upName)
	if err != nil {
		return err
	}

	// 3) Upstream Ruby Text transformieren (minimal):
	//    class Abseil < Formula  -> class GovAbseil < Formula
	out := transformFormulaClass(rb, privateName)

	// 4) Header / Kontext ausgeben
	fmt.Println()
	if apply {
		fmt.Println("=== APPLY UPDATE ===")
	} else {
		fmt.Println("=== DRY-RUN UPDATE ===")
	}
	fmt.Printf("Private:  %s\n", privateName) // dein Paketname im Private Tap
	fmt.Printf("Upstream: %s\n", upName)      // upstream formula name
	fmt.Printf("Source:   %s\n", srcURL)      // URL des geladenen .rb
	fmt.Printf("Target:   %s\n", entry.Path)  // lokales Ziel-File (Mirror)
	fmt.Println()

	// 5) Mini-Sanity-Check: Prüfen, ob die erwartete Gov-Class Zeile im Output vorkommt
	//    (hilft dir zu sehen, ob transformFormulaClass korrekt gegriffen hat)
	fmt.Println("Class line check:")
	expectedLine := "class " + toGovClassName(privateName) + " < Formula"
	fmt.Printf(" - expect: %s\n", expectedLine)

	if !strings.Contains(out, expectedLine) {
		fmt.Println(" - warning: Gov class line not found after transform (check regex).")
	} else {
		fmt.Println(" - ok")
	}
	fmt.Println()

	// 6) Vorschau: erste 25 Zeilen anzeigen, damit du vor dem Schreiben kurz prüfen kannst
	fmt.Println("--- Preview (first 25 lines) ---")
	lines := strings.Split(out, "\n")
	for i := 0; i < 25 && i < len(lines); i++ {
		fmt.Println(lines[i])
	}
	fmt.Println("--- end preview ---")

	// 7) Apply-Mode: Datei wirklich überschreiben
	//    Dry-Run: nichts schreiben
	fmt.Println()
	if apply {
		// Schreibzugriff ins Mirror Repo (.cache/private-tap)
		// Hinweis: das ist NICHT automatisch gepusht/committed, nur lokal geschrieben.
		if err := os.WriteFile(entry.Path, []byte(out), 0o644); err != nil {
			return err
		}

		fmt.Println("Wrote updated file to:", entry.Path)
		fmt.Println("Next: cd .cache/private-tap && git diff")
		fmt.Println()
	} else {
		fmt.Println("Nothing was written. Run again with --apply to overwrite the file.")
		fmt.Println()
	}

	return nil
}

// fetchUpStreamRB lädt das komplette Ruby-File (.rb) als Text.
//
// Priorität:
// 1) Wenn upstreamName in externalTapRawRB vorkommt, lade von dort (externe Taps).
// 2) Sonst: Homebrew-core raw URL generieren und laden.
func fetchUpStreamRB(client *http.Client, upstreamName string) (content string, srcURL string, err error) {
	// 1) Externe Tap-Overrides (z.B. danger-js, sdkman-cli, ...)
	if raw, ok := externalTapRawRB[upstreamName]; ok {
		txt, err := httpGetText(client, raw)
		return txt, raw, err
	}

	// 2) Standard: homebrew-core raw URL
	raw := buildHomebrewCoreRawURL(upstreamName)
	txt, err := httpGetText(client, raw)
	return txt, raw, err
}

// buildHomebrewCoreRawURL baut die Raw-URL für homebrew-core.
// homebrew-core Struktur: Formula/<first-letter>/<name>.rb
func buildHomebrewCoreRawURL(name string) string {
	first := name[:1]
	return "https://raw.githubusercontent.com/Homebrew/homebrew-core/refs/heads/master/Formula/" +
		first + "/" + name + ".rb"
}

// httpGetText macht einen HTTP GET und gibt den Response Body als string zurück.
// - prüft Status Codes (404 -> not found, andere non-2xx -> Fehler)
// - liest gesamten Body in memory (bei .rb ok)
func httpGetText(client *http.Client, url string) (string, error) {
	// HTTP Request
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	// Body immer schliessen, sonst leakt man Ressourcen
	defer func() { _ = resp.Body.Close() }()

	// 404 bedeutet: Datei existiert nicht an dieser URL
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("upstream rb not found (404): %s", url)
	}
	// andere Fehlercodes sauber ausgeben
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http status %d for %s", resp.StatusCode, url)
	}

	// Body lesen
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// transformFormulaClass ersetzt die class-Zeile im Upstream File durch deine Gov-Class.
// Beispiel:
//
//	class Abseil < Formula
//
// -> class GovAbseil < Formula
//
// Regex ist bewusst tolerant: matcht "class <word> < Formula" plus evtl. trailing stuff.
func transformFormulaClass(rb string, privateName string) string {
	govClass := toGovClassName(privateName)

	// ^class <Name> < Formula ... (ganze Zeile)
	re := regexp.MustCompile(`(?m)^\s*class\s+\w+\s+<\s+Formula\b.*$`)
	return re.ReplaceAllString(rb, "class "+govClass+" < Formula")
}

// toGovClassName macht aus "gov-abseil" -> "GovAbseil"
// und aus "gov-git-filter-repo" -> "GovGitFilterRepo"
func toGovClassName(privateName string) string {
	// gov- prefix entfernen
	s := strings.TrimPrefix(privateName, "gov-")

	// in Teile splitten (an "-")
	parts := strings.Split(s, "-")

	var b strings.Builder
	b.WriteString("Gov")

	// jedes Teil in CamelCase
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		if len(p) > 1 {
			b.WriteString(p[1:])
		}
	}
	return b.String()
}
