package main

import (
	"errors"
	"fmt"
	"os"

	git "github.com/go-git/go-git/v5"
)

func main() {
	dst := ".cache/homebrew-core"
	url := "https://github.com/Homebrew/homebrew-core.git"

	if err := os.MkdirAll(dst, 0755); err != nil {
		panic(err)
	}
	_, err := os.Stat(dst)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("Cloning:", url)
		_, err := git.PlainClone(dst, false, &git.CloneOptions{
			URL:      url,
			Depth:    1,
		})
		if err != nil {
			panic(err)
		}
		fmt.Println("Done")
		return
	}
	if err != nil {
		panic(err)
	}
	fmt.Println("Already cloned:", dst)
}

