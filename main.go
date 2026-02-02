package main

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"time"
)

func main() {
	// Dein Tap Root (nicht /Formula, sondern Root)
	privateTapPath := "/opt/homebrew/Library/Taps/tlchmi/homebrew-ch-gov-brew"

	privateVers, err := loadFormulaVersions(privateTapPath)
	if err != nil {
		panic(err)
	}

	type behindRow struct {
		privateName string
		upstream    string
		privateVer  string
		upstreamVer string
	}

	var behind []behindRow
	var notFound []string
	var errorsList []string

	// HTTP Client (wiederverwenden)
	client := &http.Client{Timeout: 15 * time.Second}

	for pName, pVer := range privateVers {
		upName := toUpstreamName(pName)

		upVer, ok, err := fetchUpstreamStable(client, upName)
		if err != nil {
			errorsList = append(errorsList, fmt.Sprintf("%s -> %s: %v", pName, upName, err))
			continue
		}
		if !ok {
			notFound = append(notFound, pName)
			continue
		}

		if isBehind(pVer, upVer) {
			behind = append(behind, behindRow{
				privateName: pName,
				upstream:    upName,
				privateVer:  pVer,
				upstreamVer: upVer,
			})
		}
	}

	sort.Slice(behind, func(i, j int) bool { return behind[i].privateName < behind[j].privateName })
	sort.Strings(notFound)
	sort.Strings(errorsList)

	fmt.Printf("Private Tap Formulae (found Version): %d\n", len(privateVers))
	fmt.Printf("Behind homebrew/core: %d\n", len(behind))
	fmt.Printf("Not found in homebrew/core (404): %d\n", len(notFound))
	fmt.Printf("HTTP/Parse Error: %d\n\n", len(errorsList))

	if len(behind) > 0 {
		fmt.Println("=== Behind Upstream (Please update) ===")
		for _, r := range behind {
			fmt.Printf("- %s (upstream: %s): %s -> %s\n", r.privateName, r.upstream, r.privateVer, r.upstreamVer)
		}
		fmt.Println()
	}

	if len(notFound) > 0 {
		fmt.Println("=== Not found in homebrew/core (first 25) ===")
		for i := 0; i < 25 && i < len(notFound); i++ {
			fmt.Printf("- %s (searching upstream: %s)\n", notFound[i], toUpstreamName(notFound[i]))
		}
		fmt.Println()
	}

	if len(errorsList) > 0 {
		fmt.Println("=== Errors (first 10) ===")
		for i := 0; i < 10 && i < len(errorsList); i++ {
			fmt.Printf("- %s\n", errorsList[i])
		}
		fmt.Println()
	}

	// Für CI: Exit Code 2 wenn etwas hinterher ist
	if len(behind) > 0 {
		os.Exit(2)
	}
}
