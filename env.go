package main

import (
	"bufio"
	"os"
	"strings"
)

// loadDotEnv lädt einfache KEY=VALUE Zeilen aus einer Datei in os.Environ,
// ohne bestehende Env-Variablen zu überschreiben.
// - Kommentare (# ...) und leere Zeilen werden ignoriert.
// - Werte können optional in "..." oder '...' stehen.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		//Wenn datei nicht existiert: kein Fehler
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Quotes entfernen
		val = strings.Trim(val, `"'`)

		// Nicht überschreiben, wenn schon gesetzt
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return sc.Err()
}
