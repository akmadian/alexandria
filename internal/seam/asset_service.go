package seam

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/log"

	"github.com/akmadian/alexandria/internal/ast"
	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
)

// assetReader is the read slice of the asset repository the seam needs — a
// capability-scoped local interface (not the concrete repo), so the service is
// unit-testable without a database. Mirrors the query-layer methods on
// catalog.AssetReader, so sqlite.AssetRepo satisfies it.
type assetReader interface {
	Get(ctx context.Context, id string) (*domain.Asset, error)
	QueryAssets(ctx context.Context, query ast.Query, arrangement ast.Arrangement, page ast.Page) ([]catalog.AssetRow, int, error)
	AssetIDSlice(ctx context.Context, query ast.Query, arrangement ast.Arrangement, fromIndex, toIndex int) ([]string, error)
	IndexOfAsset(ctx context.Context, query ast.Query, arrangement ast.Arrangement, id string) (*int, error)
	DistinctValues(ctx context.Context, field ast.Field) ([]string, error)
}

// assetJudgmentWriter is the user-action write slice: the only path that bumps
// judgment_modified_at. Matches catalog.AssetJudgmentWriter.
type assetJudgmentWriter interface {
	ApplyTriagePatch(ctx context.Context, ids []string, p catalog.TriagePatch) error
	ApplyTriagePatchByQuery(ctx context.Context, query ast.Query, exceptIDs []string, p catalog.TriagePatch) ([]string, error)
	SoftDelete(ctx context.Context, ids []string) error
}

// enrichmentView is the seam's read slice of the engine (task 21): per-asset
// transient + failed state as KIND NAMES, resolved a page at a time (the
// enrichment.Engine satisfies it). Optional — a nil view leaves rows undecorated,
// so the read path works with no engine (every existing test, and any host built
// before enrichment is wired). It never leaks the engine's internal bitmask.
type enrichmentView interface {
	RunningKinds(assetIDs []string) map[string][]domain.EnrichmentKind
	FailedKinds(ctx context.Context, assetIDs []string) (map[string][]domain.EnrichmentKind, error)
}

// enrichmentDecoratable is the setter WithEnrichmentView targets — only
// AssetService implements it, so the option is a no-op on any other service.
type enrichmentDecoratable interface {
	setEnrichmentView(view enrichmentView)
}

// AssetService exposes the asset read + judgment-write surface — the workhorse of
// the seam (C7): one QueryAssets absorbs every predicate, one UpdateAssets absorbs
// every triage write. It reads through the query authority (ast) and writes
// through the judgment writer; it holds no business logic itself. Successful
// writes emit catalog/changed (C8) so the frontend invalidates — the mutation
// choke point is the natural producer (impl/16 §4).
type AssetService struct {
	emitting
	reader     assetReader
	writer     assetJudgmentWriter
	enrichment enrichmentView // nil = no decoration
}

func (s *AssetService) setEnrichmentView(view enrichmentView) { s.enrichment = view }

// NewAssetService constructs the bound service over the asset read/write slices.
// Pass WithEmitter to wire catalog/changed events; without it, writes succeed and
// emit nothing (the nil-safe default) — which is what the existing tests rely on.
func NewAssetService(reader assetReader, writer assetJudgmentWriter, opts ...Option) *AssetService {
	service := &AssetService{reader: reader, writer: writer}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

// WithEnrichmentView wires the engine's per-asset visibility into the asset
// service, so query results carry enrichment decoration (task 21). Only
// AssetService consumes it — the option type-asserts for the setter, so it is a
// no-op on any other bound service.
func WithEnrichmentView(view enrichmentView) Option {
	return func(s serviceOption) {
		if decoratable, ok := s.(enrichmentDecoratable); ok {
			decoratable.setEnrichmentView(view)
		}
	}
}

// QueryResult is the QueryAssets envelope: the grid-card page plus the total
// matching the query (ignoring paging) that sizes the scrollbar. Total is a new
// result SHAPE, not a new predicate — the same query drives both.
type QueryResult struct {
	Items []catalog.AssetRow `json:"items"`
	Total int                `json:"total"`
}

// UpdateTarget is the mutation target: explicit ids, or "everything matching this
// query except these ids". The query form compiles to ONE statement in the engine
// (… AND id NOT IN (…)), never an id-materialized loop. Destructive DISK ops would
// take ids only — but this target feeds triage/soft-delete, which are reversible,
// so the query form is allowed here (C7 / §Additions).
type UpdateTarget struct {
	IDs       []string   `json:"ids,omitempty"`
	Query     *ast.Query `json:"query,omitempty"`
	ExceptIDs []string   `json:"exceptIds,omitempty"`
}

// TriagePatchInput is the sparse triage patch as it arrives over the wire. Each
// field is raw JSON so the seam can distinguish the three Opt states the frontend
// encodes as `field?: value | null`: absent key = don't touch, explicit null =
// clear, value = set (mirrors domain.Opt / catalog.TriagePatch).
//
// ponytail: raw-JSON fields decode the three states correctly but generate a loose
// TS type; the final wire encoding is settled in the deferred contract.ts
// reconciliation (needs the frontend types in hand), tracked in DEFERRED §7.
type TriagePatchInput struct {
	Rating     json.RawMessage `json:"rating,omitempty"`
	ColorLabel json.RawMessage `json:"colorLabel,omitempty"`
	Flag       json.RawMessage `json:"flag,omitempty"`
	Note       json.RawMessage `json:"note,omitempty"`
}

// QueryAssets returns one page of the working set plus its total. It validates the
// query before touching the repo (row #1) so a malformed tree returns query_invalid
// rather than a SQL error, and a version-too-new tree returns query_version_too_new.
func (s *AssetService) QueryAssets(query ast.Query, arrangement ast.Arrangement, page ast.Page) (QueryResult, error) {
	if err := ast.Validate(query); err != nil {
		return QueryResult{}, normalizeError(err)
	}
	items, total, err := s.reader.QueryAssets(seamContext(), query, arrangement, page)
	if err != nil {
		log.Error("seam: QueryAssets failed", "err", err)
		return QueryResult{}, normalizeError(err)
	}
	s.decorateEnrichment(seamContext(), items)
	log.Debug("seam: queried assets", "returned", len(items), "total", total)
	return QueryResult{Items: items, Total: total}, nil
}

// decorateEnrichment fills each row's transient enrichment state (task 21): the
// running kinds (one tracker lock for the page) and the terminally-failed kinds
// (one DLQ query). Nil-safe (no engine wired = no decoration) and best-effort —
// decoration never fails a query, so a DLQ read error drops the failed half and
// logs, leaving the running half intact.
func (s *AssetService) decorateEnrichment(ctx context.Context, rows []catalog.AssetRow) {
	if s.enrichment == nil || len(rows) == 0 {
		return
	}
	ids := make([]string, len(rows))
	for index := range rows {
		ids[index] = rows[index].ID
	}
	running := s.enrichment.RunningKinds(ids)
	failed, err := s.enrichment.FailedKinds(ctx, ids)
	if err != nil {
		log.Warn("seam: enrichment failed-state decoration skipped", "err", err)
	}
	for index := range rows {
		rows[index].Enriching = running[rows[index].ID]
		rows[index].Failed = failed[rows[index].ID]
	}
}

// GetAsset returns the full-asset detail projection by id — the inspector's
// read (the grid uses the slim AssetRow projection). The wire shape is
// AssetDetail, not *domain.Asset: the seam decides which fields cross.
func (s *AssetService) GetAsset(id string) (*AssetDetail, error) {
	asset, err := s.reader.Get(seamContext(), id)
	if err != nil {
		log.Error("seam: GetAsset failed", "id", id, "err", err)
		return nil, normalizeError(err)
	}
	detail := detailFromAsset(asset)
	log.Debug("seam: got asset", "id", id)
	return &detail, nil
}

// AssetIDSlice returns the ids in [fromIndex, toIndex) of the compiled ordering —
// the range-selection materialization (§Additions). Ids-only so a range over a
// large working set never ships rows.
func (s *AssetService) AssetIDSlice(query ast.Query, arrangement ast.Arrangement, fromIndex, toIndex int) ([]string, error) {
	if err := ast.Validate(query); err != nil {
		return nil, normalizeError(err)
	}
	ids, err := s.reader.AssetIDSlice(seamContext(), query, arrangement, fromIndex, toIndex)
	if err != nil {
		log.Error("seam: AssetIDSlice failed", "err", err)
		return nil, normalizeError(err)
	}
	log.Debug("seam: id slice", "from", fromIndex, "to", toIndex, "count", len(ids))
	return ids, nil
}

// IndexOfAsset returns the asset's index in the compiled ordering, or null if it
// is not in the working set — the cursor keep-if-present primitive (§Additions).
func (s *AssetService) IndexOfAsset(query ast.Query, arrangement ast.Arrangement, id string) (*int, error) {
	if err := ast.Validate(query); err != nil {
		return nil, normalizeError(err)
	}
	index, err := s.reader.IndexOfAsset(seamContext(), query, arrangement, id)
	if err != nil {
		log.Error("seam: IndexOfAsset failed", "id", id, "err", err)
		return nil, normalizeError(err)
	}
	log.Debug("seam: index of asset", "id", id, "found", index != nil)
	return index, nil
}

// DistinctValues returns the distinct values of a suggestable field, powering the
// parser and editor suggestions (§Additions). The field name is validated against
// the grammar so an unknown or non-suggestable field returns query_invalid rather
// than reaching the repo.
func (s *AssetService) DistinctValues(field string) ([]string, error) {
	spec, ok := ast.LookupField(ast.Field(field))
	if !ok {
		return nil, normalizeError(&ast.ErrUnknownField{Field: ast.Field(field)})
	}
	if !spec.Suggestable {
		return nil, normalizeError(&ast.ErrInvalidValue{
			Field:   ast.Field(field),
			Message: "field is not suggestable",
		})
	}
	values, err := s.reader.DistinctValues(seamContext(), ast.Field(field))
	if err != nil {
		log.Error("seam: DistinctValues failed", "field", field, "err", err)
		return nil, normalizeError(err)
	}
	log.Debug("seam: distinct values", "field", field, "count", len(values))
	return values, nil
}

// UpdateAssets applies a sparse triage patch to the target (ids, or query minus
// exceptIds). Absent fields are untouched, explicit nulls clear. The query form is
// validated and compiled to a single statement; an empty target is a validation
// error rather than a silent no-op over everything.
//
//nolint:gocritic // hugeParam: patch is a seam request DTO deserialized by value at the Wails boundary; a pointer would admit nil for a required arg.
func (s *AssetService) UpdateAssets(target UpdateTarget, patch TriagePatchInput) error {
	triage, err := patch.toTriagePatch()
	if err != nil {
		return normalizeError(err)
	}
	// An empty patch touches nothing. Guard it per-branch (not before the target
	// switch) so an empty *target* still validation-errors below regardless of the
	// patch, while a valid target with nothing to write is a true no-op that
	// neither writes nor emits (impl/16 §6: a no-op does not emit).
	emptyPatch := triage == (catalog.TriagePatch{})

	switch {
	case len(target.IDs) > 0:
		if emptyPatch {
			log.Debug("seam: UpdateAssets no-op (empty patch)", "ids", len(target.IDs))
			return nil
		}
		if err := s.writer.ApplyTriagePatch(seamContext(), target.IDs, triage); err != nil {
			log.Error("seam: UpdateAssets by ids failed", "count", len(target.IDs), "err", err)
			return normalizeError(err)
		}
		log.Info("seam: updated assets by ids", "count", len(target.IDs))
		s.emit(EventCatalogChanged, CatalogChange{Scope: ScopeAssets})
		return nil
	case target.Query != nil:
		if err := ast.Validate(*target.Query); err != nil {
			return normalizeError(err)
		}
		if emptyPatch {
			log.Debug("seam: UpdateAssets no-op (empty patch)")
			return nil
		}
		affected, err := s.writer.ApplyTriagePatchByQuery(seamContext(), *target.Query, target.ExceptIDs, triage)
		if err != nil {
			log.Error("seam: UpdateAssets by query failed", "err", err)
			return normalizeError(err)
		}
		log.Info("seam: updated assets by query", "affected", len(affected))
		// Only a write that moved rows is a change worth invalidating on.
		if len(affected) > 0 {
			s.emit(EventCatalogChanged, CatalogChange{Scope: ScopeAssets})
		}
		return nil
	default:
		return normalizeError(&domain.ValidationError{
			Field:   "target",
			Message: "update target requires either ids or a query",
		})
	}
}

// RemoveFromCatalog soft-deletes the given assets (reversible; the row stays for
// undo). Destructive on-disk deletion is a separate, unbuilt engine capability —
// see DEFERRED §7.
func (s *AssetService) RemoveFromCatalog(ids []string) error {
	if len(ids) == 0 {
		log.Debug("seam: RemoveFromCatalog no-op (no ids)")
		return nil
	}
	if err := s.writer.SoftDelete(seamContext(), ids); err != nil {
		log.Error("seam: RemoveFromCatalog failed", "count", len(ids), "err", err)
		return normalizeError(err)
	}
	log.Info("seam: removed assets from catalog", "count", len(ids))
	s.emit(EventCatalogChanged, CatalogChange{Scope: ScopeAssets})
	return nil
}

// toTriagePatch decodes the three-state wire patch into the engine's TriagePatch.
func (p *TriagePatchInput) toTriagePatch() (catalog.TriagePatch, error) {
	rating, err := decodeOpt[int](p.Rating)
	if err != nil {
		return catalog.TriagePatch{}, fmt.Errorf("rating: %w", err)
	}
	colorLabel, err := decodeOpt[domain.ColorLabel](p.ColorLabel)
	if err != nil {
		return catalog.TriagePatch{}, fmt.Errorf("colorLabel: %w", err)
	}
	flag, err := decodeOpt[domain.Flag](p.Flag)
	if err != nil {
		return catalog.TriagePatch{}, fmt.Errorf("flag: %w", err)
	}
	note, err := decodeOpt[string](p.Note)
	if err != nil {
		return catalog.TriagePatch{}, fmt.Errorf("note: %w", err)
	}
	return catalog.TriagePatch{Rating: rating, ColorLabel: colorLabel, Flag: flag, Note: note}, nil
}

// decodeOpt turns a raw JSON patch field into a domain.Opt: absent (empty) means
// "don't touch", explicit null means "clear", any other value means "set". A
// malformed value is a validation error (surfaces as domain/validation).
func decodeOpt[T any](raw json.RawMessage) (domain.Opt[T], error) {
	if len(raw) == 0 {
		return domain.Opt[T]{}, nil
	}
	if string(raw) == "null" {
		return domain.ClearOpt[T](), nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return domain.Opt[T]{}, &domain.ValidationError{Field: "patch", Message: err.Error()}
	}
	return domain.SetOpt(value), nil
}
