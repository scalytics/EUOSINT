// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/scalytics/euosint/internal/collector/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Fatal(err)
	}
}
