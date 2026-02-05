package main

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"time"
)

type behindRow struct {
	privateName string
	upstream    string
	privateVer  string
	upstreamVer string
}

type report struct {
	privateCount int
	behind       []behindRow
	notFound     []string
	errorsList   []string
}

func main() {
	os.Exit(run())
}
func run() int {
	tapURL := "https://bitbucket.bit.admin.ch/scm/tlchmi/homebrew-ch-gov-brew.git"

	privateTapPath, err := ensureRepoMirror(".cache/private-tap", tapURL)
	if err != nil {
		panic(err)
	}

	privateVers, err := loadFormulaVersions(privateTapPath)
	if err != nil {
		panic(err)
	}

	client := &http.Client{Timeout: 15 * time.Second}

	rep := compareAll(client, privateVers)
	printReport(privateVers, rep)

	if len(rep.behind) > 0 {
		return 2 // CI signal: something is behind upstream
	}
	return 0
}

func compareAll(client *http.Client, privateVers map[string]string) report {
	var rep report

	for pName, pVer := range privateVers {
		upName := toUpstreamName(pName)

		upVer, ok, err := fetchUpstreamStable(client, upName)
		if err != nil {
			rep.errorsList = append(rep.errorsList, fmt.Sprintf("%s -> %s: %v", pName, upName, err))
			continue
		}
		if !ok {
			rep.notFound = append(rep.notFound, pName)
			continue
		}
		if isBehind(pVer, upVer) {
			rep.behind = append(rep.behind, behindRow{
				privateName: pName,
				upstream:    upName,
				privateVer:  pVer,
				upstreamVer: upVer,
			})
		}
	}
	sort.Slice(rep.behind, func(i, j int) bool { return rep.behind[i].privateName < rep.behind[j].privateName })
	sort.Strings(rep.notFound)
	sort.Strings(rep.errorsList)

	return rep
}

func printReport(privateVers map[string]string, rep report) {
	fmt.Printf("Private Tap Formulae (found Version): %d\n", len(privateVers))
	fmt.Printf("Behind upstream: %d\n", len(rep.behind))
	fmt.Printf("Not found upstream: %d\n", len(rep.notFound))
	fmt.Printf("HTTP/Parse Error: %d\n\n", len(rep.errorsList))

	if len(rep.behind) > 0 {
		fmt.Printf("=== Behind Upstream (Please update) ===\n")
		for _, r := range rep.behind {
			fmt.Printf(" - %s (upstream: %s): %s -> %s\n", r.privateName, r.upstream, r.privateVer, r.upstreamVer)
		}
		fmt.Println()
	}

	if len(rep.notFound) > 0 {
		fmt.Println("=== Not found Upstream (firts 15) ===")
		for i := 0; i < 25 && i < len(rep.notFound); i++ {
			fmt.Printf("- %s (searching upstream: %s\n", rep.notFound[i], toUpstreamName(rep.notFound[i]))
		}
		fmt.Println()
	}
	if len(rep.errorsList) > 0 {
		fmt.Println("=== Errors (first 10) ===")
		for i := 0; i < 10 && i < len(rep.errorsList); i++ {
			fmt.Printf("- %s\n", rep.errorsList[i])
		}
		fmt.Println()
	}
}
