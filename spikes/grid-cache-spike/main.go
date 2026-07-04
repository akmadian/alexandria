// Grid cache spike — validates the two open risks from docs/frontend-architecture.md §6:
//  1. Does the webview HTTP cache honor Cache-Control on the Wails custom scheme?
//     (Measured: asset-handler hits on scroll-down vs scroll-back-up.)
//  2. Does the virtualized grid scroll smoothly inside the real webview (not Chromium)?
//     (Measured: rAF frame deltas during programmatic fling.)
//
// MUST be run as a production build (go build -tags desktop,production) — `wails dev`
// serves over real HTTP and sidesteps the custom-scheme question entirely.
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	gocontext "context"
)

//go:embed all:frontend/dist
var assets embed.FS

const thumbDir = "thumbs"

var (
	thumbCount = envInt("THUMBS", 3000)
	manualMode = os.Getenv("MANUAL") == "1"
	avgKB      float64

	totalHits  atomic.Int64
	uniqueHits sync.Map // path -> *atomic.Int64
	uniqueN    atomic.Int64
)

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return def
}

// ---- thumbnail generation: 256px JPEGs, flat color blocks + light noise ≈ 25–45KB ----

func generateThumbs() error {
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		return err
	}
	// Skip if already generated for this count.
	if _, err := os.Stat(filepath.Join(thumbDir, fmt.Sprintf("%d.jpg", thumbCount-1))); err == nil {
		var total int64
		for i := 0; i < thumbCount; i++ {
			if st, err := os.Stat(filepath.Join(thumbDir, fmt.Sprintf("%d.jpg", i))); err == nil {
				total += st.Size()
			}
		}
		avgKB = float64(total) / float64(thumbCount) / 1024
		return nil
	}

	var total atomic.Int64
	var wg sync.WaitGroup
	work := make(chan int)
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				n, err := writeThumb(i)
				if err != nil {
					fmt.Fprintf(os.Stderr, "thumb %d: %v\n", i, err)
					continue
				}
				total.Add(n)
			}
		}()
	}
	for i := 0; i < thumbCount; i++ {
		work <- i
	}
	close(work)
	wg.Wait()
	avgKB = float64(total.Load()) / float64(thumbCount) / 1024
	return nil
}

func writeThumb(i int) (int64, error) {
	rng := rand.New(rand.NewSource(int64(i)))
	const size, block = 256, 16
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for by := 0; by < size; by += block {
		for bx := 0; bx < size; bx += block {
			base := color.RGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255}
			for y := by; y < by+block; y++ {
				for x := bx; x < bx+block; x++ {
					n := int8(rng.Intn(41) - 20) // noise tuned so JPEG size ≈ real photo thumbnails (~30KB)
					img.SetRGBA(x, y, color.RGBA{clamp(base.R, n), clamp(base.G, n), clamp(base.B, n), 255})
				}
			}
		}
	}
	f, err := os.Create(filepath.Join(thumbDir, fmt.Sprintf("%d.jpg", i)))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 85}); err != nil {
		return 0, err
	}
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

func clamp(v uint8, d int8) uint8 {
	r := int(v) + int(d)
	if r < 0 {
		return 0
	}
	if r > 255 {
		return 255
	}
	return uint8(r)
}

// ---- asset handler: /thumb/{i}.jpg with immutable caching + hit counting ----

type thumbHandler struct{}

func (thumbHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if !strings.HasPrefix(path, "thumb/") {
		http.NotFound(w, r)
		return
	}
	totalHits.Add(1)
	c, loaded := uniqueHits.LoadOrStore(r.URL.Path, new(atomic.Int64))
	if !loaded {
		uniqueN.Add(1)
	}
	c.(*atomic.Int64).Add(1)

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, filepath.Join(thumbDir, strings.TrimPrefix(path, "thumb/")))
}

// ---- bindings ----

type App struct{ ctx gocontext.Context }

type Stats struct {
	Total  int64 `json:"total"`
	Unique int64 `json:"unique"`
}

type Config struct {
	Count  int     `json:"count"`
	Manual bool    `json:"manual"`
	AvgKB  float64 `json:"avgKB"`
}

func (a *App) GetConfig() Config { return Config{Count: thumbCount, Manual: manualMode, AvgKB: avgKB} }
func (a *App) GetStats() Stats   { return Stats{Total: totalHits.Load(), Unique: uniqueN.Load()} }

func (a *App) LogResult(result string) {
	var pretty map[string]any
	out := result
	if json.Unmarshal([]byte(result), &pretty) == nil {
		if b, err := json.MarshalIndent(pretty, "", "  "); err == nil {
			out = string(b)
		}
	}
	fmt.Println("=== SPIKE RESULT ===")
	fmt.Println(out)
	_ = os.WriteFile("results.json", []byte(out+"\n"), 0o644)
}

func (a *App) Quit() { wruntime.Quit(a.ctx) }

func main() {
	if err := generateThumbs(); err != nil {
		fmt.Fprintln(os.Stderr, "thumbnail generation failed:", err)
		os.Exit(1)
	}
	fmt.Printf("spike: %d thumbnails, avg %.1f KB, manual=%v\n", thumbCount, avgKB, manualMode)

	app := &App{}
	err := wails.Run(&options.App{
		Title:  "grid-cache-spike",
		Width:  1200,
		Height: 900,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: thumbHandler{},
		},
		OnStartup: func(ctx gocontext.Context) { app.ctx = ctx },
		Bind:      []interface{}{app},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
