package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

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
	return strings.TrimPrefix(private, "gov-")
}

// ---- Upstream stable Version via API holen ----
func fetchUpstreamStable(client *http.Client, formula string) (stable string, ok bool, err error) {
	url := "https://formulae.brew.sh/api/formula/" + formula + ".json"

	resp, err := client.Get(url)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", false, nil
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
