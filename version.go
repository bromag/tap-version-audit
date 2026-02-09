package main

import (
	"strings" // für Trim/Replace Operationen an Version Strings

	// hashicorp/go-version ist eine Library, die semantische Versionen vergleichen kann.
	// Vorteil: "1.10.0" wird korrekt als neuer als "1.9.0" erkannt.
	goversion "github.com/hashicorp/go-version"
)

// normalizeVersion bereitet Version-Strings so auf, dass sie möglichst gut
// von go-version geparst werden können.
//
// Beispiele:
// " v1.2.3 "   -> "1.2.3"
// "1_2_3"      -> "1.2.3"
// "1,2,3"      -> "1.2.3"
//
// Warum:
// In Formulae kommen Versionen manchmal mit "v" prefix, mit "_" oder ",".
// go-version erwartet normalerweise ein sauberes "1.2.3".
func normalizeVersion(s string) string {
	// Leerzeichen entfernen
	s = strings.TrimSpace(s)

	// Häufiges Prefix entfernen: "v1.2.3" -> "1.2.3"
	s = strings.TrimPrefix(s, "v")

	// Manche Versionen verwenden "_" statt ".", z.B. "1_2_3"
	s = strings.ReplaceAll(s, "_", ".")

	// Manche Versionen kommen mit Kommas, z.B. "1,2,3"
	s = strings.ReplaceAll(s, ",", ".")

	return s
}

// isBehind vergleicht "local" (deine Version) mit "upstream" (Homebrew stable).
//
// Rückgabe:
// true  -> local ist älter als upstream (du bist "behind")
// false -> local ist gleich oder neuer (oder Vergleich nicht möglich)
//
// Ablauf:
// 1) Beide Versionen normalisieren (normalizeVersion)
// 2) versuchen, sie als Version-Objekte zu parsen (go-version)
// 3) wenn beide parsebar sind: sauber vergleichen
// 4) sonst: Fallback-Vergleich (sehr einfach, nicht perfekt)
func isBehind(local, upstream string) bool {
	// 1) Local Version parsen
	vl, el := goversion.NewVersion(normalizeVersion(local))

	// 2) Upstream Version parsen
	vu, eu := goversion.NewVersion(normalizeVersion(upstream))

	// 3) Wenn beide erfolgreich geparst wurden, machen wir einen echten Versionsvergleich
	//    Beispiel: 1.10.0 > 1.9.0 wird korrekt erkannt
	if el == nil && eu == nil {
		return vl.LessThan(vu)
	}

	// 4) Fallback (MVP):
	//    Wenn parsing bei einer oder beiden Versionen nicht klappt,
	//    vergleichen wir die Strings direkt.
	//
	// Achtung: Das ist NICHT zuverlässig für alle Fälle.
	// Beispiel: "1.10.0" < "1.9.0" wäre als String-Vergleich falsch.
	// Wenn du willst, können wir hier später einen robusteren "natural compare" bauen
	// oder Sonderfälle normalisieren (z.B. rc/beta, datums-versionen, etc.).
	return local < upstream
}
