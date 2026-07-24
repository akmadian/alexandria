package ast

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// ErrVersionTooNew means the query was written by a newer app version.
type ErrVersionTooNew struct {
	Got  int
	Want int
}

func (e *ErrVersionTooNew) Error() string {
	return fmt.Sprintf("query version %d is newer than supported version %d", e.Got, e.Want)
}

// --- Query ---

type queryJSON struct {
	Version int              `json:"version"`
	Scope   *scopeJSON       `json:"scope,omitempty"`
	Where   *json.RawMessage `json:"where,omitempty"`
}

type scopeJSON struct {
	Kind      ScopeKind `json:"kind"`
	ID        string    `json:"id,omitempty"`
	VolumeID  string    `json:"volumeId,omitempty"`
	Path      string    `json:"path,omitempty"`
	Recursive bool      `json:"recursive,omitempty"`
}

func (q Query) MarshalJSON() ([]byte, error) {
	out := queryJSON{Version: q.Version}
	if q.Scope != nil {
		out.Scope = &scopeJSON{
			Kind:      q.Scope.Kind,
			ID:        q.Scope.ID,
			VolumeID:  q.Scope.VolumeID,
			Path:      q.Scope.Path,
			Recursive: q.Scope.Recursive,
		}
	}
	if q.Where != nil {
		raw, err := marshalNode(q.Where)
		if err != nil {
			return nil, err
		}
		msg := json.RawMessage(raw)
		out.Where = &msg
	}
	return json.Marshal(out)
}

func (q *Query) UnmarshalJSON(data []byte) error {
	// Reject unknown keys.
	dec := json.NewDecoder(bytesReader(data))
	dec.DisallowUnknownFields()
	var raw queryJSON
	if err := dec.Decode(&raw); err != nil {
		return fmt.Errorf("query: %w", err)
	}
	if raw.Version == 0 {
		return fmt.Errorf("query: missing or zero version")
	}
	if raw.Version > Version {
		return &ErrVersionTooNew{Got: raw.Version, Want: Version}
	}
	q.Version = raw.Version
	if raw.Scope != nil {
		q.Scope = &Scope{
			Kind:      raw.Scope.Kind,
			ID:        raw.Scope.ID,
			VolumeID:  raw.Scope.VolumeID,
			Path:      raw.Scope.Path,
			Recursive: raw.Scope.Recursive,
		}
	}
	if raw.Where != nil {
		node, err := unmarshalNode(*raw.Where)
		if err != nil {
			return fmt.Errorf("query.where: %w", err)
		}
		q.Where = node
	}
	return nil
}

// --- Node ---

type groupJSON struct {
	Op       GroupOp            `json:"op"`
	Children []*json.RawMessage `json:"children"`
}

type leafJSON struct {
	Field Field    `json:"field"`
	Cmp   Operator `json:"cmp"`
	Value any      `json:"value,omitempty"`
}

func marshalNode(node Node) ([]byte, error) {
	switch typed := node.(type) {
	case Group:
		children := make([]*json.RawMessage, len(typed.Children))
		for i, child := range typed.Children {
			encoded, err := marshalNode(child)
			if err != nil {
				return nil, err
			}
			msg := json.RawMessage(encoded)
			children[i] = &msg
		}
		return json.Marshal(groupJSON{Op: typed.Op, Children: children})
	case Leaf:
		return json.Marshal(leafJSON{Field: typed.Field, Cmp: typed.Cmp, Value: marshalLeafValue(typed)})
	default:
		return nil, fmt.Errorf("unknown node type %T", node)
	}
}

func marshalLeafValue(leaf Leaf) any {
	if leaf.Value == nil {
		return nil
	}
	switch v := leaf.Value.(type) {
	case DateValue:
		return marshalDateValue(&v)
	default:
		return v
	}
}

// dateValueJSON is the wire form of DateValue (docs/frontend-architecture.md + the ISO 8601
// decision, 2026-07-10): anchor is "now" | RFC 3339 timestamp | date-only
// "2006-01-02" (local midnight); duration is an ISO 8601 duration string.
type dateValueJSON struct {
	Anchor   string `json:"anchor"`
	Duration string `json:"duration"`
}

// anchorNow is the symbolic anchor resolved to `now` at compile time.
const anchorNow = "now"

// dateOnlyLayout is the anchor's date-only form, interpreted as local
// midnight ("today" means the user's today — value.go Resolve).
const dateOnlyLayout = "2006-01-02"

func marshalDateValue(date *DateValue) dateValueJSON {
	out := dateValueJSON{Duration: FormatISODuration(date.Duration)}
	if date.Anchor.Now {
		out.Anchor = anchorNow
	} else {
		out.Anchor = date.Anchor.Date.Format(time.RFC3339)
	}
	return out
}

func unmarshalNode(data json.RawMessage) (Node, error) {
	// Peek at keys to dispatch: "op" → Group, "field" → Leaf, both/neither → error.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("node: %w", err)
	}
	_, hasOp := probe["op"]
	_, hasField := probe["field"]
	if hasOp && hasField {
		return nil, fmt.Errorf("node: ambiguous — has both 'op' and 'field'")
	}
	if !hasOp && !hasField {
		return nil, fmt.Errorf("node: missing both 'op' and 'field'")
	}

	if hasOp {
		return unmarshalGroup(data)
	}
	return unmarshalLeaf(data)
}

func unmarshalGroup(data json.RawMessage) (Group, error) {
	dec := json.NewDecoder(bytesReader(data))
	dec.DisallowUnknownFields()
	var raw groupJSON
	if err := dec.Decode(&raw); err != nil {
		return Group{}, fmt.Errorf("group: %w", err)
	}
	children := make([]Node, len(raw.Children))
	for i, child := range raw.Children {
		node, err := unmarshalNode(*child)
		if err != nil {
			return Group{}, fmt.Errorf("group.children[%d]: %w", i, err)
		}
		children[i] = node
	}
	return Group{Op: raw.Op, Children: children}, nil
}

func unmarshalLeaf(data json.RawMessage) (Leaf, error) {
	dec := json.NewDecoder(bytesReader(data))
	dec.DisallowUnknownFields()
	var raw leafJSON
	if err := dec.Decode(&raw); err != nil {
		return Leaf{}, fmt.Errorf("leaf: %w", err)
	}

	value, err := coerceLeafValue(raw.Field, raw.Cmp, raw.Value)
	if err != nil {
		return Leaf{}, fmt.Errorf("leaf %s %s: %w", raw.Field, raw.Cmp, err)
	}
	return Leaf{Field: raw.Field, Cmp: raw.Cmp, Value: value}, nil
}

// coerceLeafValue converts JSON-decoded values to the Go types the compiler
// expects. JSON numbers arrive as float64; string arrays may arrive as
// []interface{}. DateValue objects arrive as nested maps.
func coerceLeafValue(field Field, cmp Operator, raw any) (any, error) {
	if raw == nil {
		return nil, nil
	}
	spec, ok := LookupField(field)
	if !ok {
		return raw, nil // validation will catch the unknown field
	}

	switch spec.Kind {
	case KindDateRange:
		return coerceDateValue(raw)
	case KindEnum, KindEntityReference:
		return coerceStringOrSlice(raw)
	default:
		return raw, nil
	}
}

func coerceStringOrSlice(raw any) (any, error) {
	switch v := raw.(type) {
	case string:
		return v, nil
	case []any:
		result := make([]string, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string in array, got %T", item)
			}
			result[i] = s
		}
		return result, nil
	default:
		return raw, nil
	}
}

func coerceDateValue(raw any) (DateValue, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return DateValue{}, err
	}
	var decoded dateValueJSON
	if err := json.Unmarshal(b, &decoded); err != nil {
		return DateValue{}, fmt.Errorf("dateValue: %w", err)
	}

	var anchor DateAnchor
	switch decoded.Anchor {
	case anchorNow:
		anchor.Now = true
	case "":
		return DateValue{}, fmt.Errorf("dateValue: missing anchor")
	default:
		parsed, err := parseAnchor(decoded.Anchor)
		if err != nil {
			return DateValue{}, fmt.Errorf("dateValue anchor: %w", err)
		}
		anchor.Date = parsed
	}

	duration, err := ParseISODuration(decoded.Duration)
	if err != nil {
		return DateValue{}, fmt.Errorf("dateValue: %w", err)
	}
	return DateValue{Anchor: anchor, Duration: duration}, nil
}

// parseAnchor accepts an RFC 3339 timestamp or a date-only form (local
// midnight — day boundaries follow the machine's timezone).
func parseAnchor(raw string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed, nil
	}
	return time.ParseInLocation(dateOnlyLayout, raw, time.Local)
}

func bytesReader(data []byte) io.Reader {
	return bytes.NewReader(data)
}
