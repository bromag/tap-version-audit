package main

import (
	"flag"     // CLI Flags wie --update und --apply
	"fmt"      // Ausgabe im Terminal
	"net/http" // HTTP Client für Upstream API Requests
	"os"       // Env Variablen, Exit Codes
	"sort"     // Sortieren der Ergebnislisten
	"time"     // Timeout für HTTP
)

// behindRow beschreibt einen Eintrag, der in deinem Private Tap "hinterher" ist
// (d.h. upstream ist neuer als deine Version).
type behindRow struct {
	privateName string // z.B. "gov-abseil"
	upstream    string // z.B. "abseil"
	privateVer  string // Version aus deinem Tap (z.B. aus url/ version Zeile)
	upstreamVer string // Version aus formulae.brew.sh (stable)
	privatePath string // lokaler Pfad zur Datei im Mirror (.cache/private-tap/...)
}

// report sammelt alle Resultate eines Runs, damit wir sie am Ende schön ausgeben können.
type report struct {
	privateCount int // (optional) Anzahl private formulae; du verwendest aktuell len(privateEntries)
	behind       []behindRow
	notFound     []string // private packages, die upstream nicht gefunden wurden (404)
	errorsList   []string // HTTP / Parse / sonstige Fehler (nicht fatal, aber loggen)
}

func main() {
	// main gibt den Rückgabecode von run() zurück (wichtig für CI)
	os.Exit(run())
}

func run() int {
	// 1) .env laden (optional)
	//    Zweck: TAP_URL / BITBUCKET_USER / BITBUCKET_TOKEN etc. automatisch ins Environment laden
	//    Wenn die Datei nicht existiert: kein Fehler (je nachdem wie loadDotEnv implementiert ist)
	if err := loadDotEnv(".env"); err != nil {
		panic(err)
	}

	// Debug/Transparenz: Zeigt dir, ob TAP_URL überhaupt geladen wurde.
	fmt.Println("TAP_URL:", os.Getenv("TAP_URL"))

	// 2) CLI Flags definieren
	// --update gov-abseil  -> ein einziges Package "updaten" (dry-run oder apply)
	// --apply              -> wenn gesetzt: wirklich schreiben (sonst nur dry-run)
	updateName := flag.String("update", "", "dry-run update one private formula (e.g. gov-abseil)")
	apply := flag.Bool("apply", false, "write changes into the private tap mirror (no push!)")
	flag.Parse()

	// 3) TAP_URL aus ENV holen (kommt aus .env oder aus deinem Shell Environment)
	tapURL := os.Getenv("TAP_URL")
	if tapURL == "" {
		panic("TAP_URL environment variable not set")
	}

	// 4) Private Tap Mirror sicherstellen:
	//    - falls .cache/private-tap noch nicht existiert: clone (shallow)
	//    - falls existiert: pull (main/master fallback)
	//    Damit vergleichst du nicht gegen dein lokales /opt/homebrew/... Tap,
	//    sondern gegen den aktuellen Stand von Bitbucket.
	privateTapPath, err := ensureRepoMirror(".cache/private-tap", tapURL)
	if err != nil {
		panic(err)
	}

	// 5) Aus dem lokalen Mirror alle Formula Files scannen und Version + Pfad extrahieren
	//    Ergebnis: map[name]localFormula, z.B. "gov-abseil" -> {Version:"...", Path:".../gov-abseil.rb"}
	privateEntries, err := loadFormulaEntries(privateTapPath)
	if err != nil {
		panic(err)
	}

	// 6) HTTP Client erstellen (wiederverwenden, damit nicht pro Request ein neuer Client gebaut wird)
	// Timeout verhindert "hängenbleiben", wenn upstream langsam ist.
	client := &http.Client{Timeout: 15 * time.Second}

	// 7) Vergleich machen: deine Version vs upstream stable Version
	rep := compareAll(client, privateEntries)

	// 8) Report ausgeben (behind, notfound, errors)
	printReport(privateEntries, rep)

	// 9) Optional: Update-Mode für ein einzelnes Package (z.B. gov-abseil)
	//    Wichtig: in diesem Mode wollen wir NICHT mit Exit Code 2 rausgehen,
	//    weil du es lokal testest und nur ein Update ansehen willst.
	if *updateName != "" {
		// Existiert dieses Package überhaupt in deinem privateEntries Map?
		e, ok := privateEntries[*updateName]
		if !ok {
			panic("unknown private formula: " + *updateName)
		}

		// Führt Dry-Run oder Apply aus:
		// - dryRunUpdateOne(..., apply=false) -> zeigt nur Preview, schreibt nichts
		// - dryRunUpdateOne(..., apply=true)  -> schreibt ins Mirror File, aber macht kein commit/push
		if err := updateOne(client, *updateName, e, *apply); err != nil {
			panic(err)
		}

		// Update-Mode = immer 0, damit dein Terminal nicht mit exit status 2 endet.
		return 0
	}

	// 10) CI Signal: Wenn irgendetwas hinterher ist, geben wir 2 zurück.
	//     Das ist hilfreich, wenn du es später in Pipelines laufen lässt.
	if len(rep.behind) > 0 {
		return 2
	}

	// 11) Alles aktuell -> Exit 0
	return 0
}

func compareAll(client *http.Client, privateEntries map[string]localFormula) report {
	// Wir bauen das report Objekt zusammen und liefern es zurück.
	var rep report

	// Loop über alle privaten Formulae
	for pName, e := range privateEntries {
		// lokale Version (aus deinem Parser)
		pVer := e.Version

		// privateName -> upstreamName (gov-foo@... -> foo / overrides etc.)
		upName := toUpstreamName(pName)

		// Upstream stable Version holen (über formulae.brew.sh API, plus fallback taps falls eingebaut)
		upVer, ok, err := fetchUpstreamStable(client, upName)
		if err != nil {
			// Fehler bei HTTP/JSON/Parsing -> wir sammeln es, aber brechen nicht alles ab
			rep.errorsList = append(rep.errorsList, fmt.Sprintf("%s -> %s: %v", pName, upName, err))
			continue
		}
		if !ok {
			// Upstream nicht gefunden (404) -> in notFound Liste aufnehmen
			rep.notFound = append(rep.notFound, pName)
			continue
		}

		// Versionsvergleich (deine Version kleiner als upstream = behind)
		if isBehind(pVer, upVer) {
			rep.behind = append(rep.behind, behindRow{
				privateName: pName,
				upstream:    upName,
				privateVer:  pVer,
				upstreamVer: upVer,
				privatePath: e.Path, // extrem wichtig fürs spätere Apply/Overwrite
			})
		}
	}

	// Ergebnislisten sortieren, damit Output reproduzierbar ist
	sort.Slice(rep.behind, func(i, j int) bool { return rep.behind[i].privateName < rep.behind[j].privateName })
	sort.Strings(rep.notFound)
	sort.Strings(rep.errorsList)

	return rep
}

func printReport(privateEntries map[string]localFormula, rep report) {
	// Summary
	fmt.Printf("Private Tap Formulae (found Version): %d\n", len(privateEntries))
	fmt.Printf("Behind upstream: %d\n", len(rep.behind))
	fmt.Printf("Not found upstream: %d\n", len(rep.notFound))
	fmt.Printf("HTTP/Parse Error: %d\n\n", len(rep.errorsList))

	// Liste der veralteten Packages
	if len(rep.behind) > 0 {
		fmt.Printf("=== Behind Upstream (Please update) ===\n")
		for _, r := range rep.behind {
			// nur Anzeige: welches Package ist alt und welche Versionen
			fmt.Printf(" - %s (upstream: %s): %s -> %s\n", r.privateName, r.upstream, r.privateVer, r.upstreamVer)
		}
		fmt.Println()
	}

	// Liste von packages, die upstream nicht gefunden wurden
	// (meist: anderer Tap, anderer Name, oder nur in cask)
	if len(rep.notFound) > 0 {
		fmt.Println("=== Not found Upstream (firts 15) ===")
		for i := 0; i < 25 && i < len(rep.notFound); i++ {
			fmt.Printf("- %s (searching upstream: %s)\n", rep.notFound[i], toUpstreamName(rep.notFound[i]))
		}
		fmt.Println()
	}

	// Fehlerliste (nur die ersten 10, damit Output nicht explodiert)
	if len(rep.errorsList) > 0 {
		fmt.Println("=== Errors (first 10) ===")
		for i := 0; i < 10 && i < len(rep.errorsList); i++ {
			fmt.Printf("- %s\n", rep.errorsList[i])
		}
		fmt.Println()
	}
}
