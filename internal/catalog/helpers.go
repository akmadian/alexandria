package catalog

import (
	"database/sql"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := formatTime(*t)
	return &s
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func parseTimePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

func parseNullTime(ns sql.NullString) *time.Time {
	if !ns.Valid {
		return nil
	}
	return parseTimePtr(ns.String)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullStringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}

func nullIntPtr(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	v := int(ni.Int64)
	return &v
}

func nullFloat64Ptr(nf sql.NullFloat64) *float64 {
	if !nf.Valid {
		return nil
	}
	return &nf.Float64
}

func checkRowsAffected(res sql.Result, resource, id string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return &domain.NotFoundError{Resource: resource, ID: id}
	}
	return nil
}
