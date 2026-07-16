package main

// The enrichment observability surface (task 22, D28 commitment #2 and #4): the
// `jobs graph` subcommand renders the registry graph, and `import --debug` mounts
// a live domain-vocabulary page (asset / kind / artifact / queue) over the
// engine's Snapshot — never a generic dump, the pprof anti-lesson. The snapshot
// is served raw at /enrichment/snapshot.json too: that JSON is the contract the
// in-app dev corner will consume later; this page is just its first reader.

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/akmadian/alexandria/internal/assettype"
	"github.com/akmadian/alexandria/internal/enrichment"
	"github.com/akmadian/alexandria/internal/sqlite"
)

// cmdJobs handles `dev jobs graph` — the flat registry rendered as a per-asset-
// type hierarchy (D28 commitment #2). DOT by default (pipe to `dot -Tsvg`),
// ASCII with --format=ascii for a terminal.
func cmdJobs(args []string) error {
	if len(args) < 1 || args[0] != "graph" {
		return fmt.Errorf("usage: dev jobs graph [--format dot|ascii]")
	}
	flags := flag.NewFlagSet("graph", flag.ExitOnError)
	format := flags.String("format", "dot", "dot | ascii")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	definitions := enrichment.Definitions(nil, nil) // structure only; producers never run
	switch *format {
	case "dot":
		fmt.Print(enrichment.RenderGraphDOT(definitions, assettype.All()))
	case "ascii":
		fmt.Print(enrichment.RenderGraphASCII(definitions, assettype.All()))
	default:
		return fmt.Errorf("unknown format %q (want dot or ascii)", *format)
	}
	return nil
}

// mountEnrichmentDebug registers the enrichment page and its JSON feed on the
// default mux (before startDebugServer starts serving). It reads the live engine
// snapshot plus a bounded catalog slice for the asset × kind matrix.
func mountEnrichmentDebug(rig *enrichmentRig, catalog *openedCatalog) {
	http.HandleFunc("/enrichment/snapshot.json", func(writer http.ResponseWriter, request *http.Request) {
		snapshot, err := rig.engine.Snapshot(request.Context())
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(snapshot); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
	})
	http.HandleFunc("/enrichment", func(writer http.ResponseWriter, request *http.Request) {
		snapshot, err := rig.engine.Snapshot(request.Context())
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		matrix, err := buildMatrix(request.Context(), catalog, rig.definitions, snapshot.InFlight, matrixLimit)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := enrichmentPageTemplate.Execute(writer, newPageView(&snapshot, matrix)); err != nil {
			// Header may already be flushed; a log line is all we can do.
			fmt.Println("enrichment debug page:", err)
		}
	})
	// The control surface: debug-mode convenience actions over the existing engine
	// verbs (task 21's PauseAll/ResumeAll). The SHIPPED controls go through the
	// seam, not here — this is the dev page driving the same verbs for the
	// inspect-then-approve gate. POST-only + redirect so a refresh can't re-fire.
	http.HandleFunc("/enrichment/pause", engineAction(func() { rig.engine.PauseAll() }))
	http.HandleFunc("/enrichment/resume", engineAction(func() { rig.engine.ResumeAll() }))
}

// engineAction wraps an engine verb as a POST-only handler that redirects back to
// the page (so the meta-refresh picks up the new state).
func engineAction(verb func()) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "POST only", http.StatusMethodNotAllowed)
			return
		}
		verb()
		http.Redirect(writer, request, "/enrichment", http.StatusSeeOther)
	}
}

// matrixLimit bounds the asset × kind matrix — the most-recent N assets. A dev
// fixture is small; this only guards against opening the page on a real catalog.
const matrixLimit = 200

// --- Page view models --------------------------------------------------------

type pageView struct {
	Snapshot enrichment.Snapshot
	InFlight []inFlightRow
	Matrix   matrixView
}

type inFlightRow struct {
	Asset  string
	Kind   string
	Age    string
	Hinted bool
}

type matrixView struct {
	Kinds []string
	Rows  []matrixRow
}

type matrixRow struct {
	Asset string
	Ext   string
	Cells []string // one state per kind, aligned with Kinds: done|running|failed|pending|na
}

func newPageView(snapshot *enrichment.Snapshot, matrix matrixView) pageView {
	now := time.Now()
	inFlight := make([]inFlightRow, 0, len(snapshot.InFlight))
	for _, job := range snapshot.InFlight {
		inFlight = append(inFlight, inFlightRow{
			Asset:  shortID(job.AssetID),
			Kind:   job.Kind,
			Age:    now.Sub(job.Started).Round(time.Millisecond).String(),
			Hinted: job.Hinted,
		})
	}
	return pageView{Snapshot: *snapshot, InFlight: inFlight, Matrix: matrix}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// buildMatrix assembles the asset × kind matrix for the most-recent assets: done
// from the artifact columns (catalog truth), running from the live snapshot,
// failed from the attempt-exhausted DLQ, pending otherwise. "queued" collapses
// into pending on purpose — the missing artifact IS the queue (D28), and the
// per-kind queue depth is shown as an aggregate instead of dumped per cell.
func buildMatrix(ctx context.Context, catalog *openedCatalog, definitions []enrichment.JobDefinition, inFlight []enrichment.InFlightJob, limit int) (matrixView, error) {
	// Build the presence SELECT from the definitions' artifact columns. Each is
	// validated against the derived-column allowlist before interpolation (the
	// same guard the repo uses), so the query carries no untrusted input.
	kinds := make([]string, len(definitions))
	var query strings.Builder
	query.WriteString("SELECT id, extension")
	for index := range definitions {
		column := definitions[index].ArtifactColumn
		if !sqlite.IsDerivedArtifactColumn(column) {
			return matrixView{}, fmt.Errorf("matrix: %q is not a derived artifact column", column)
		}
		kinds[index] = definitions[index].Kind
		query.WriteString(", ")
		query.WriteString(column)
		query.WriteString(" IS NOT NULL")
	}
	query.WriteString(" FROM assets WHERE is_deleted = 0 ORDER BY ingested_at DESC LIMIT ?")
	rows, err := catalog.store.DB.QueryContext(ctx, query.String(), limit)
	if err != nil {
		return matrixView{}, err
	}
	defer rows.Close()

	type assetPresence struct {
		id, ext string
		present []bool
	}
	var assets []assetPresence
	var assetIDs []string
	for rows.Next() {
		present := make([]bool, len(definitions))
		scanTargets := make([]any, 0, len(definitions)+2)
		var id, ext string
		scanTargets = append(scanTargets, &id, &ext)
		for index := range present {
			scanTargets = append(scanTargets, &present[index])
		}
		if err := rows.Scan(scanTargets...); err != nil {
			return matrixView{}, err
		}
		assets = append(assets, assetPresence{id: id, ext: ext, present: present})
		assetIDs = append(assetIDs, id)
	}
	if err := rows.Err(); err != nil {
		return matrixView{}, err
	}

	repo := &sqlite.EnrichmentRepo{DB: catalog.store.DB}
	exhausted, err := repo.ExhaustedKinds(ctx, assetIDs, enrichment.MaxAttempts)
	if err != nil {
		return matrixView{}, err
	}
	running := make(map[string]bool, len(inFlight))
	for _, job := range inFlight {
		running[job.AssetID+"\x00"+job.Kind] = true
	}

	view := matrixView{Kinds: kinds, Rows: make([]matrixRow, 0, len(assets))}
	for _, asset := range assets {
		handler, known := assettype.Classify(asset.ext)
		cells := make([]string, len(definitions))
		for index := range definitions {
			cells[index] = cellState(&definitions[index], handler, known, asset.present[index],
				running[asset.id+"\x00"+definitions[index].Kind],
				containsKind(exhausted[asset.id], definitions[index].Kind))
		}
		view.Rows = append(view.Rows, matrixRow{Asset: shortID(asset.id), Ext: asset.ext, Cells: cells})
	}
	return view, nil
}

// cellState resolves one asset×kind cell. Running wins (it is happening now);
// then a present artifact reads done (a failed state never outlives its artifact,
// D28); then attempt-exhausted reads failed; otherwise pending. An inapplicable
// kind is n/a.
func cellState(definition *enrichment.JobDefinition, handler assettype.Handler, known, present, running, exhausted bool) string {
	if !known || !definition.Applicable(handler) {
		return "na"
	}
	switch {
	case running:
		return "running"
	case present:
		return "done"
	case exhausted:
		return "failed"
	default:
		return "pending"
	}
}

func containsKind(kinds []string, kind string) bool {
	for _, candidate := range kinds {
		if candidate == kind {
			return true
		}
	}
	return false
}

// enrichmentPageTemplate is the live page — one self-contained HTML doc that
// meta-refreshes every second (stdlib only, no JS/SSE: DEFERRED §9's constraint
// stands). Cells and gauges speak asset / kind / artifact / queue.
var enrichmentPageTemplate = template.Must(template.New("enrichment").Funcs(template.FuncMap{
	"pct": func(part, whole int64) int64 {
		if whole == 0 {
			return 0
		}
		return part * 100 / whole
	},
}).Parse(`<!doctype html><meta charset=utf-8><title>Alexandria enrichment</title>
<meta http-equiv="refresh" content="1">
<style>
 body{font:13px/1.4 ui-monospace,monospace;margin:1.2rem;color:#111}
 h2{font-size:14px;margin:1.4rem 0 .4rem;border-bottom:1px solid #ccc}
 table{border-collapse:collapse;margin:.2rem 0}
 th,td{padding:2px 8px;text-align:left;border:1px solid #ddd}
 th{background:#f4f4f4}
 .bar{display:inline-block;height:12px;background:#4a90d9}
 .done{background:#d7f0d7}.running{background:#fff3c4}.failed{background:#f6c9c9}.pending{background:#eee;color:#888}.na{color:#ccc}
 .flag{color:#4a90d9}
 form.ctl{display:inline;margin-right:.4rem}
 button{font:inherit;padding:2px 10px;cursor:pointer}
 .paused{color:#b00;font-weight:bold}
</style>
<h1>enrichment engine — live</h1>
<p>effort: <b>{{.Snapshot.Effort}}</b>
 {{if .Snapshot.Paused}}· <span class=paused>PAUSED (all)</span>{{else}}· dispatching{{end}}
 · budget {{.Snapshot.Budget.InUse}}/{{.Snapshot.Budget.Usable}} in use (capacity {{.Snapshot.Budget.Capacity}})
 <span class="bar" style="width:{{pct .Snapshot.Budget.InUse .Snapshot.Budget.Capacity}}px"></span></p>
<p>
 <form class=ctl method=post action="/enrichment/pause"><button {{if .Snapshot.Paused}}disabled{{end}}>Pause all</button></form>
 <form class=ctl method=post action="/enrichment/resume"><button {{if not .Snapshot.Paused}}disabled{{end}}>Resume all</button></form>
</p>

<h2>queues</h2>
<table>
 <tr><th>kind</th><th>hot</th><th>cold</th><th>running</th><th>workers</th><th>more?</th><th>paused?</th></tr>
 {{range .Snapshot.Kinds}}<tr>
  <td>{{.Kind}}</td><td>{{.QueuedHot}}</td><td>{{.QueuedCold}}</td>
  <td>{{.Running}}</td><td>{{.Workers}}</td>
  <td>{{if .More}}…{{end}}</td><td>{{if .Paused}}paused{{end}}</td>
 </tr>{{end}}
</table>

<h2>in-flight ({{len .InFlight}})</h2>
<table>
 <tr><th>asset</th><th>kind</th><th>age</th><th></th></tr>
 {{range .InFlight}}<tr><td>{{.Asset}}</td><td>{{.Kind}}</td><td>{{.Age}}</td><td>{{if .Hinted}}<span class=flag>hint</span>{{end}}</td></tr>{{end}}
</table>

<h2>DLQ by reason</h2>
<table>
 <tr><th>kind</th><th>reason</th><th>count</th><th>exhausted</th></tr>
 {{range .Snapshot.DLQ}}<tr><td>{{.Kind}}</td><td>{{.Reason}}</td><td>{{.Count}}</td><td>{{.Exhausted}}</td></tr>{{end}}
</table>

<h2>asset × kind (most recent {{len .Matrix.Rows}})</h2>
<table>
 <tr><th>asset</th><th>ext</th>{{range .Matrix.Kinds}}<th>{{.}}</th>{{end}}</tr>
 {{range .Matrix.Rows}}<tr><td>{{.Asset}}</td><td>{{.Ext}}</td>{{range .Cells}}<td class="{{.}}">{{.}}</td>{{end}}</tr>{{end}}
</table>
`))
