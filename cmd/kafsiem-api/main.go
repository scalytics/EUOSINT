package main

import (
	"log"
	"os"

	"github.com/scalytics/kafSIEM/internal/kafsiemapi"
)

func main() {
	if err := kafsiemapi.RunMain(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Fatal(err)
	}
}
