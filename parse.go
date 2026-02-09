package main

import (
	"io/fs"         // Typen für WalkDir Callback (fs.DirEntry)
	"os"            // File lesen
	"path"          // URL/Path handling (path.Base für URL-Pfade)
	"path/filepath" // OS-spezifische Pfade (Join, WalkDir)
	"regexp"        // Regex für url/version parsing
	"strings"       // Strings trimmen, suffix prüfen etc.
)

// ---- Regex für Version / URL in Formula Ruby Files ----
//
// Hinweis: du hast reVersion aktuell auskommentiert.
// Das bedeutet: du liest NICHT "version ..." Zeilen,
// sondern leitest die Version nur aus der "url ..." ab.
//
// var reVersion = regexp.MustCompile(`(?m)^\s*version(?:\s*\(\s*)?\s*["']([^"']+)["']`)
var reURL = regexp.MustCompile(`(?m)^\s*url\s+["']([^"']+)["']`)

// localFormula beschreibt eine local tap formula, die wir gefunden haben.
// - Version: extrahierte Version (z.B. 3.14.2 oder 20260107.0)
// - Path: absoluter/relativer Pfad zum Ruby File in deinem Mirror/Repo
type localFormula struct {
	Version string
	Path    string
}

// loadFormulaEntries läuft durch repoPath/Formula und sammelt alle .rb Dateien.
// Für jede Datei wird versucht, eine Version zu extrahieren.
// Rückgabe:
// map[formulaName]localFormula
//
//	z.B. "gov-abseil" -> {Version:"20260107.0", Path:".../Formula/a/gov-abseil.rb"}
func loadFormulaEntries(repoPath string) (map[string]localFormula, error) {
	// Formel-Verzeichnis (Homebrew-typisch: <tap>/Formula)
	formulaDir := filepath.Join(repoPath, "Formula")

	// Output Map initialisieren
	out := map[string]localFormula{}

	// WalkDir traversiert rekursiv alle Dateien/Ordner im Formula-Verzeichnis
	err := filepath.WalkDir(formulaDir, func(p string, d fs.DirEntry, err error) error {
		// Wenn WalkDir selbst auf einen Fehler läuft, müssen wir ihn zurückgeben,
		// sonst werden Files "übersprungen" und du merkst es nicht.
		if err != nil {
			return err
		}

		// Wir interessieren uns nur für Dateien, nicht Ordner
		// und nur Ruby formula files (*.rb)
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".rb") {
			return nil
		}

		// formula Name ist der Filename ohne .rb
		// Beispiel: gov-abseil.rb -> gov-abseil
		name := strings.TrimSuffix(d.Name(), ".rb")

		// Datei Inhalt einlesen
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}

		// Version extrahieren (aktuell: aus URL)
		v := extractVersion(string(b), name)

		// Nur aufnehmen, wenn wir wirklich eine Version gefunden haben
		if v != "" {
			out[name] = localFormula{
				Version: v, // extrahierte Version
				Path:    p, // Pfad zum File (wichtig fürs Update/Overwrite)
			}
		}
		return nil
	})

	// WalkDir Fehler (oder nil) zurückgeben
	return out, err
}

// extractVersion versucht aus dem Ruby File Content eine Version zu extrahieren.
// Deine aktuelle Logik:
// 1) version Zeile ist auskommentiert
// 2) url Zeile matchen und daraus eine Version ableiten
//
// Vorteil: funktioniert bei vielen Formulae, weil die URL meistens die Version enthält.
// Nachteil: manchmal ist die Version nicht sauber in der URL oder endet mit ".tar" etc.
func extractVersion(content, pkgName string) string {
	// Wenn du später wieder "version ..." direkt unterstützen willst,
	// kannst du den reVersion Block wieder aktivieren:
	//
	// if m := reVersion.FindStringSubmatch(content); len(m) == 2 {
	//     return strings.TrimSpace(m[1])
	// }

	// 2) Sonst: url "..." Zeile finden
	if m := reURL.FindStringSubmatch(content); len(m) == 2 {
		// m[1] ist die URL aus der url "..."" Zeile
		return inferVersionFromURL(m[1], pkgName)
	}

	// Wenn weder version noch url gefunden wird: keine Version
	return ""
}

// stripExt entfernt bekannte Archiv-Endungen aus einem string.
// Beispiel:
// "foo-1.2.3.tar.gz" -> "foo-1.2.3"
// "foo-1.2.3.zip"    -> "foo-1.2.3"
//
// Das ist wichtig, damit wir danach die Version aus dem Dateinamen ziehen können.
func stripExt(s string) string {
	exts := []string{
		".tar.gz", ".tar.bz2", ".tar.xz", ".tgz",
		".zip", ".gz", ".bz2", ".xz",
		".tar",
	}

	// Wenn bekannte Endung vorhanden: wegtrimmen
	for _, e := range exts {
		if strings.HasSuffix(s, e) {
			return strings.TrimSuffix(s, e)
		}
	}

	// Fallback: nur die letzte Extension entfernen (z.B. .tgz schon abgedeckt, aber zur Sicherheit)
	if i := strings.LastIndex(s, "."); i > 0 {
		return s[:i]
	}

	// Keine Extension erkannt: unverändert zurück
	return s
}

// inferVersionFromURL versucht aus einer URL eine Version zu erraten.
// Beispiel:
// url: https://github.com/abseil/abseil-cpp/archive/refs/tags/20260107.0.tar.gz
// -> Version: 20260107.0
//
// Vorgehen:
// 1) base filename aus URL holen (path.Base)
// 2) Archiv-Endung entfernen (stripExt)
// 3) mehrere Kandidaten versuchen (mit/ohne pkgName prefix)
// 4) Regex sucht nach "v?1.2.3..." Mustern
func inferVersionFromURL(u, pkgName string) string {
	// Strings trimmen, dann basename (letzter Teil der URL) holen
	// und Archiv-Endung entfernen
	base := stripExt(path.Base(strings.TrimSpace(u)))

	// Kandidaten: manchmal steht der Name vorne dran:
	// name-1.2.3, name_1.2.3, namev1.2.3
	// Darum versuchen wir:
	// - base wie er ist
	// - base ohne "pkgName-" prefix
	// - base ohne "pkgName_" prefix
	// - base ohne "pkgNamev" prefix
	candidates := []string{
		base,
		strings.TrimPrefix(base, pkgName+"-"),
		strings.TrimPrefix(base, pkgName+"_"),
		strings.TrimPrefix(base, pkgName+"v"),
	}

	// Regex für Versionstoken:
	// - optional "v"
	// - mindestens "digit.digit" (also 1.2)
	// - erlaubt danach noch suffix (rc, beta, etc.) via [A-Za-z0-9._-]*
	reVerToken := regexp.MustCompile(`v?(\d+(?:\.\d+)+[A-Za-z0-9._-]*)`)

	// 1) Kandidaten prüfen
	for _, c := range candidates {
		// FindStringSubmatch gibt:
		// m[0] = gesamter match
		// m[1] = captured group (die Version, die wir wollen)
		if m := reVerToken.FindStringSubmatch(c); len(m) == 2 {
			// Falls captured group mit "v" beginnt: entfernen
			return strings.TrimPrefix(m[1], "v")
		}
	}

	// 2) Fallback: wenn im Filename nichts gefunden wurde,
	// suchen wir nochmal im gesamten URL-String (manchmal steckt die Version nicht im basename)
	if m := reVerToken.FindStringSubmatch(u); len(m) == 2 {
		return strings.TrimPrefix(m[1], "v")
	}

	// Nichts gefunden
	return ""
}
