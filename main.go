package main

import (
	"log"

	"github.com/bnema/git-sync/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}