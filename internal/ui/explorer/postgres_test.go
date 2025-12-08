package explorer

import (
	"testing"
	"time"
)

func TestQuoteIdent(t *testing.T) {
	cases := map[string]string{
		`simple`:       `"simple"`,
		`needs"escape`: `"needs""escape"`,
		``:             `""`,
	}
	for input, expected := range cases {
		got := quoteIdent(input)
		if got != expected {
			t.Fatalf("quoteIdent(%q) = %q, expected %q", input, got, expected)
		}
	}
}

func TestFormatCell(t *testing.T) {
	now := time.Now()
	if formatCell(nil) != "NULL" {
		t.Fatalf("expected NULL for nil value")
	}
	if formatCell([]byte("data")) != "data" {
		t.Fatalf("expected byte slices to convert to string")
	}
	if formatCell(now) != now.Format(time.RFC3339) {
		t.Fatalf("expected RFC3339 formatting for time values")
	}
}

func TestIsSelectStatement(t *testing.T) {
	tests := map[string]bool{
		"SELECT * FROM foo": true,
		"  select 1":        true,
		"with data as ()":   true,
		"update foo":        false,
		"delete from foo":   false,
	}

	for sql, expected := range tests {
		if got := isSelectStatement(sql); got != expected {
			t.Fatalf("isSelectStatement(%q)=%v, expected %v", sql, got, expected)
		}
	}
}
