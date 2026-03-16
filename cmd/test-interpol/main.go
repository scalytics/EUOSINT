package main

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/scalytics/euosint/internal/collector/config"
	"github.com/scalytics/euosint/internal/collector/fetch"
)

func main() {
	cfg := config.Default()
	client := fetch.New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Fetch ICPC wanted page
	body, err := client.Text(ctx, "https://icpc.gov.ng/wanted-persons/", true, "text/html")
	if err != nil {
		fmt.Println("ERROR:", err)
		return
	}
	s := string(body)
	fmt.Printf("Got %d bytes\n", len(s))
	// Find person-related patterns
	for _, pattern := range []string{"wanted", "person", "card", "grid", "profile", "name", "offence", "offense"} {
		idx := 0
		count := 0
		lower := strings.ToLower(s)
		for {
			i := strings.Index(lower[idx:], pattern)
			if i < 0 { break }
			idx += i + len(pattern)
			count++
		}
		if count > 0 {
			fmt.Printf("  '%s' found %d times\n", pattern, count)
		}
	}
	// Show a chunk around "wanted" context
	lower := strings.ToLower(s)
	if i := strings.Index(lower, "wanted-person"); i >= 0 {
		start := i - 200
		if start < 0 { start = 0 }
		end := i + 500
		if end > len(s) { end = len(s) }
		fmt.Println("\n--- CONTEXT ---")
		fmt.Println(s[start:end])
	}
}
