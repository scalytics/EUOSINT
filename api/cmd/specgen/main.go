package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/scalytics/kafSIEM/api/specgen"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	root = filepath.Clean(filepath.Join(root, ".."))
	for _, output := range specgen.Outputs() {
		target := filepath.Join(root, output.Path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(output.Content), 0o644); err != nil {
			log.Fatal(err)
		}
	}
}
