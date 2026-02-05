package main

import (
	"errors"
	"fmt"
	"os"

	// go-git ist eine reine Go-Implementation von Git (clone/pull ohne externes git binary)
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"                        // Branch/Reference-Namen, Low-Level Git-Objekte
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http" // HTTP BasicAuth für private Repos
)

// ensureRepoMirror stellt sicher, dass du lokal unter `dst` immer ein aktuelles Checkout deines
// Bitbucket-Repos hast.
//
// Ablauf:
// 1) Cache-Ordner anlegen (.cache)
// 2) Auth aus Env-Variablen holen (BITBUCKET_USER / BITBUCKET_TOKEN)
// 3) Prüfen ob dst existiert
//   - wenn nein: clone (main, sonst fallback master)
//   - wenn ja: pull (main, sonst fallback master)
//
// Rückgabe:
// - string: Pfad zum lokalen Mirror-Ordner (dst)
// - error: falls etwas schiefgeht
func ensureRepoMirror(dst, url string) (string, error) {
	// Basiscache-Ordner anlegen (falls nicht vorhanden)
	if err := os.MkdirAll(".cache", 0o755); err != nil {
		return "", err
	}

	// Credentials aus Env lesen (private Bitbucket Repos benötigen Auth)
	auth, err := bitbucketAuthFromEnv()
	if err != nil {
		return "", err
	}

	// Prüfen ob der Zielordner (Mirror) schon existiert
	exists, err := pathExists(dst)
	if err != nil {
		return "", err
	}

	// Wenn Mirror noch nicht existiert: Repository clonen
	if !exists {
		if err := cloneWithFallback(dst, url, auth); err != nil {
			return "", err
		}
		return dst, nil
	}

	// Wenn Mirror existiert: Repository aktualisieren (pull)
	if err := pullWithFallback(dst, auth); err != nil {
		_ = os.RemoveAll(dst)
		if err2 := cloneWithFallback(dst, url, auth); err2 != nil {
			return "", err2
		}
	}

	return dst, nil
}

// bitbucketAuthFromEnv baut ein HTTP BasicAuth Objekt aus Environment Variablen.
//
// Erwartet:
// - BITBUCKET_USER  (dein Username; bei go-git muss Username gesetzt sein)
// - BITBUCKET_TOKEN (PAT/AppPassword/Passwort)
//
// Rückgabe:
// - *githttp.BasicAuth: Auth-Objekt für go-git HTTP Zugriff
// - error: wenn Variablen fehlen
func bitbucketAuthFromEnv() (*githttp.BasicAuth, error) {
	user := os.Getenv("BITBUCKET_USER")
	token := os.Getenv("BITBUCKET_TOKEN")

	if user == "" || token == "" {
		return nil, fmt.Errorf("missing BITBUCKET_USER / BITBUCKET_TOKEN env vars")
	}

	return &githttp.BasicAuth{
		Username: user,
		Password: token,
	}, nil
}

// pathExists prüft, ob ein Pfad existiert.
//
// Rückgabe:
// - true, nil  -> Pfad existiert
// - false, nil -> Pfad existiert nicht
// - false, err -> Fehler beim Stat (z.B. Permission)
func pathExists(p string) (bool, error) {
	_, err := os.Stat(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// cloneWithFallback versucht das Repo zuerst vom Branch "main" zu clonen.
// Falls das fehlschlägt (z.B. weil es den Branch nicht gibt), versucht es "master".
func cloneWithFallback(dst, url string, auth *githttp.BasicAuth) error {
	// 1) main versuchen
	if err := cloneBranch(dst, url, auth, "main"); err == nil {
		return nil
	}
	// 2) fallback: master versuchen
	return cloneBranch(dst, url, auth, "master")
}

// cloneBranch clont ein Repo in den Ordner `dst`.
//
// Parameter:
// - dst: Zielordner für Clone
// - url: Repo URL (https://... .git)
// - auth: HTTP Auth für private Repos
// - branch: Branchname ("main" oder "master")
//
// Hinweise:
// - SingleBranch: nur ein Branch, spart Zeit/Traffic
// - Depth: 1 => shallow clone (nur aktuellster Stand), sehr schnell
func cloneBranch(dst, url string, auth *githttp.BasicAuth, branch string) error {
	_, err := git.PlainClone(dst, false, &git.CloneOptions{
		URL:           url,
		Auth:          auth,
		SingleBranch:  true,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		Depth:         1,
	})
	return err
}

// pullWithFallback öffnet das bestehende Repo in `dst` und macht ein Pull.
//
// Logik:
// - zuerst Pull von "main"
// - wenn das nicht klappt: Pull von "master"
// - NoErrAlreadyUpToDate ist OK (heisst: nichts zu tun)
func pullWithFallback(dst string, auth *githttp.BasicAuth) error {
	// Existierendes Repo öffnen
	repo, err := git.PlainOpen(dst)
	if err != nil {
		return err
	}

	// Worktree holen (Arbeitsverzeichnis des Repos)
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	// 1) main versuchen
	errMain := pullBranch(wt, auth, "main")
	if errMain == nil || errors.Is(errMain, git.NoErrAlreadyUpToDate) {
		return nil
	}
	// 2) fallback: master versuchen
	errMaster := pullBranch(wt, auth, "master")
	if errMaster == nil || errors.Is(errMaster, git.NoErrAlreadyUpToDate) {
		return nil
	}

	return fmt.Errorf("pull main failed: %v; pull master failed: %v", errMain, errMaster)
	// Wenn beide Branch-Pulls fehlschlagen
}

// pullBranch macht ein `git pull` für einen bestimmten Branch.
//
// Parameter:
// - wt: Worktree des geöffneten Repos
// - auth: HTTP Auth
// - branch: Branchname
//
// Hinweise:
// - Depth: 1 => shallow pull, schnell
// - SingleBranch: true => nur diesen Branch aktualisieren
func pullBranch(wt *git.Worktree, auth *githttp.BasicAuth, branch string) error {
	return wt.Pull(&git.PullOptions{
		RemoteName:    "origin",
		Auth:          auth,
		SingleBranch:  true,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		Depth:         1,
	})
}
