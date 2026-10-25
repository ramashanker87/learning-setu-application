package store

import (
	"context"
	"encoding/json"
	"errors"
)

type Doc = map[string]any

func S(d Doc, k string) string { x, _ := d[k].(string); return x }
func N(d Doc, k string) float64 {
	switch x := d[k].(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		n, _ := x.Float64()
		return n
	}
	return 0
}
func B(d Doc, k string) bool { x, _ := d[k].(bool); return x }
func M(d Doc, k string) Doc {
	x, _ := d[k].(map[string]any)
	if x == nil {
		return Doc{}
	}
	return x
}
func Clone(d Doc) Doc { b, _ := json.Marshal(d); var x Doc; _ = json.Unmarshal(b, &x); return x }

var ErrConflict = errors.New("record changed; refresh and retry")

type Write struct {
	Table         string
	Item          Doc
	Check, Delete bool
}

func Put(t string, d Doc) Write { return Write{Table: t, Item: d} }

type Repository interface {
	Get(context.Context, string, string, string) (Doc, error)
	Query(context.Context, string, string, bool) ([]Doc, error)
	QueryFields(context.Context, string, string, bool, []string) ([]Doc, error)
	Commit(context.Context, ...Write) error
	EnsureTable(context.Context, string, string) error
	OwnedTableExists(context.Context, string, string) (bool, error)
	DeleteTenantTable(context.Context, string, string) error
}
