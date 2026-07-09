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
	Kind ScopeKind `json:"kind"`
	ID   string    `json:"id,omitempty"`
}

func (q Query) MarshalJSON() ([]byte, error) {
	out := queryJSON{Version: q.Version}
	if q.Scope != nil {
		out.Scope = &scopeJSON{Kind: q.Scope.Kind, ID: q.Scope.ID}
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
		q.Scope = &Scope{Kind: raw.Scope.Kind, ID: raw.Scope.ID}
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

func marshalNode(n Node) ([]byte, error) {
	switch v := n.(type) {
	case Group:
		children := make([]*json.RawMessage, len(v.Children))
		for i, child := range v.Children {
			b, err := marshalNode(child)
			if err != nil {
				return nil, err
			}
			msg := json.RawMessage(b)
			children[i] = &msg
		}
		return json.Marshal(groupJSON{Op: v.Op, Children: children})
	case Leaf:
		val, err := marshalLeafValue(v)
		if err != nil {
			return nil, err
		}
		return json.Marshal(leafJSON{Field: v.Field, Cmp: v.Cmp, Value: val})
	default:
		return nil, fmt.Errorf("unknown node type %T", n)
	}
}

func marshalLeafValue(leaf Leaf) (any, error) {
	if leaf.Value == nil {
		return nil, nil
	}
	switch v := leaf.Value.(type) {
	case DateValue:
		return marshalDateValue(v)
	default:
		return v, nil
	}
}

type dateValueJSON struct {
	Anchor   dateAnchorJSON   `json:"anchor"`
	Duration dateDurationJSON `json:"duration"`
}

type dateAnchorJSON struct {
	Now  bool   `json:"now,omitempty"`
	Date string `json:"date,omitempty"`
}

type dateDurationJSON struct {
	Years  int `json:"years,omitempty"`
	Months int `json:"months,omitempty"`
	Days   int `json:"days,omitempty"`
}

func marshalDateValue(d DateValue) (dateValueJSON, error) {
	out := dateValueJSON{
		Duration: dateDurationJSON{Years: d.Duration.Years, Months: d.Duration.Months, Days: d.Duration.Days},
	}
	if d.Anchor.Now {
		out.Anchor.Now = true
	} else {
		out.Anchor.Date = d.Anchor.Date.Format(time.RFC3339)
	}
	return out, nil
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
	var dj dateValueJSON
	if err := json.Unmarshal(b, &dj); err != nil {
		return DateValue{}, fmt.Errorf("dateValue: %w", err)
	}

	var anchor DateAnchor
	if dj.Anchor.Now {
		anchor.Now = true
	} else if dj.Anchor.Date != "" {
		t, err := time.Parse(time.RFC3339, dj.Anchor.Date)
		if err != nil {
			return DateValue{}, fmt.Errorf("dateValue anchor: %w", err)
		}
		anchor.Date = t
	}

	return DateValue{
		Anchor:   anchor,
		Duration: DateDuration{Years: dj.Duration.Years, Months: dj.Duration.Months, Days: dj.Duration.Days},
	}, nil
}

func bytesReader(data []byte) io.Reader {
	return bytes.NewReader(data)
}
