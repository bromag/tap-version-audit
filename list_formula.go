package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

func main() {
	repoPath := "/opt/homebrew/Library/Taps/tlchmi/homebrew-ch-gov-brew"
	formulaDir := filepath.Join(repoPath, "formula")

	count := 0
	err := filepath.WalkDir(formulaDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			//Fehler beim Zugriff auf diesen Pfad z.B Rechte/kaputte Sysmlinks
			return err
		}
		//Ordner ignorieren, wir wollen nur Dateien
		if d.IsDir() {
			return nil
		}

		// Homebrew Formulae sind Ruby-Dateien (*.rb)
		if strings.HasSuffix(d.Name(), ".rb") {
			count++

			// Zum Testen nur die ersten 10 ausgeben, damit die Konsole nicht zugemüllt wird
			if count <= 10 {
				fmt.Println(d.Name())
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("Total formula files:", count)

}
