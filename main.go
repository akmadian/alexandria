// Command alexandria is the desktop application: the Wails v2 composition root.
// Wails v2 requires the main package at the module root — it runs where
// wails.json lives, and upstream declined cmd/ layouts (wails issue #2568) — so
// this is the one place it can live. main stays thin: build the app host, hand
// its bound services to Wails, run. The engine wiring lives in the host (app.go).
package main

import (
	"embed"

	"github.com/charmbracelet/log"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/akmadian/alexandria/internal/app"
)

// assets embeds the built frontend. wails dev / wails build populate
// frontend/dist first; the backend-only checks never compile this package (the
// Makefile scopes them to ./internal/... ./cmd/...), so this embed is not a
// build dependency for day-to-day engine work.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := app.SetupLogging(); err != nil {
		log.Fatalf("alexandria: logging: %v", err)
	}

	host, err := newHost()
	if err != nil {
		log.Fatalf("alexandria: startup: %v", err)
	}

	err = wails.Run(&options.App{
		Title:       "Alexandria",
		Width:       1280,
		Height:      800,
		AssetServer: &assetserver.Options{Assets: assets},
		OnStartup:   host.onStartup,
		OnShutdown:  host.onShutdown,
		Bind:        host.boundServices(),
		// No EnumBind: Wails would emit TS `enum`s, but frontend/09 mandates
		// string-literal unions. Domain enums flow through the hand-rolled
		// generator (make generate-seam) into frontend/src/_generated-types
		// instead, in the shape the frontend actually consumes (seam impl/14 §5).
	})
	if err != nil {
		log.Fatalf("alexandria: run: %v", err)
	}
}
