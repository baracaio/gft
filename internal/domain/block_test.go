package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"uuid"
)

func makeBlocksJSON(n int) string {
	var b strings.Builder
	b.WriteString(`{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","data_atualizacao":"2024-01-15T10:30:00Z"`)
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, `,"codigo_bloqueio_%d":%d,"valor_bloqueio_%d":"%d.00"`, i, i*10, i, i*10)
	}
	b.WriteString("}")
	return b.String()
}

func TestParseBlockMessage(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		want      *BlockMessage
		wantField string
	}{
		{
			name: "valid with zero blocks",
			body: `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","data_atualizacao":"2024-01-15T10:30:00Z"}`,
			want: &BlockMessage{
				Code:      uuid.MustParse(testGUID),
				Blocks:    []Block{},
				UpdatedAt: mustTime(testTS),
			},
		},
		{
			name: "valid with one block",
			body: `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","data_atualizacao":"2024-01-15T10:30:00Z","codigo_bloqueio_1":100,"valor_bloqueio_1":"150.50"}`,
			want: &BlockMessage{
				Code:      uuid.MustParse(testGUID),
				Blocks:    []Block{{Code: 100, Value: "150.50"}},
				UpdatedAt: mustTime(testTS),
			},
		},
		{
			name: "valid with integer valor",
			body: `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","data_atualizacao":"2024-01-15T10:30:00Z","codigo_bloqueio_1":100,"valor_bloqueio_1":200}`,
			want: &BlockMessage{
				Code:      uuid.MustParse(testGUID),
				Blocks:    []Block{{Code: 100, Value: "200"}},
				UpdatedAt: mustTime(testTS),
			},
		},
		{
			name: "blocks sorted by code ascending",
			body: `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","data_atualizacao":"2024-01-15T10:30:00Z","codigo_bloqueio_1":300,"valor_bloqueio_1":"30","codigo_bloqueio_2":100,"valor_bloqueio_2":"10","codigo_bloqueio_3":200,"valor_bloqueio_3":"20"}`,
			want: &BlockMessage{
				Code: uuid.MustParse(testGUID),
				Blocks: []Block{
					{Code: 100, Value: "10"},
					{Code: 200, Value: "20"},
					{Code: 300, Value: "30"},
				},
				UpdatedAt: mustTime(testTS),
			},
		},
		{
			name: "valid with 100 blocks (maximum)",
			body: makeBlocksJSON(100),
			want: &BlockMessage{
				Code:      uuid.MustParse(testGUID),
				UpdatedAt: mustTime(testTS),
			},
		},
		{
			name:      "101 blocks exceeds maximum",
			body:      makeBlocksJSON(101),
			wantField: "",
		},
		{
			name:      "invalid JSON",
			body:      `{not json}`,
			wantField: "",
		},
		{
			name:      "missing codigo_cliente",
			body:      `{"data_atualizacao":"2024-01-15T10:30:00Z"}`,
			wantField: "codigo_cliente",
		},
		{
			name:      "missing data_atualizacao",
			body:      `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000"}`,
			wantField: "data_atualizacao",
		},
		{
			name:      "invalid GUID",
			body:      `{"codigo_cliente":"not-a-guid","data_atualizacao":"2024-01-15T10:30:00Z"}`,
			wantField: "codigo_cliente",
		},
		{
			name:      "timezone-less timestamp",
			body:      `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","data_atualizacao":"2024-01-15T10:30:00"}`,
			wantField: "data_atualizacao",
		},
		{
			name:      "incomplete pair: codigo without valor",
			body:      `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","data_atualizacao":"2024-01-15T10:30:00Z","codigo_bloqueio_1":100}`,
			wantField: "valor_bloqueio_1",
		},
		{
			name:      "incomplete pair: valor without codigo",
			body:      `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","data_atualizacao":"2024-01-15T10:30:00Z","valor_bloqueio_1":"100"}`,
			wantField: "codigo_bloqueio_1",
		},
		{
			name:      "duplicate block code",
			body:      `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","data_atualizacao":"2024-01-15T10:30:00Z","codigo_bloqueio_1":100,"valor_bloqueio_1":"10","codigo_bloqueio_2":100,"valor_bloqueio_2":"20"}`,
			wantField: "",
		},
		{
			name:      "non-integer codigo_bloqueio",
			body:      `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","data_atualizacao":"2024-01-15T10:30:00Z","codigo_bloqueio_1":"abc","valor_bloqueio_1":"10"}`,
			wantField: "codigo_bloqueio_1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseBlockMessage(tc.body)

			if tc.want == nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var pErr *PermanentError
				if !errors.As(err, &pErr) {
					t.Fatalf("expected *PermanentError, got %T: %v", err, err)
				}
				if tc.wantField != "" && pErr.Field != tc.wantField {
					t.Fatalf("expected Field=%q, got %q", tc.wantField, pErr.Field)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Code != tc.want.Code {
				t.Errorf("Code: got %v, want %v", got.Code, tc.want.Code)
			}
			if !got.UpdatedAt.Equal(tc.want.UpdatedAt) {
				t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, tc.want.UpdatedAt)
			}
			if got.UpdatedAt.Location() != time.UTC {
				t.Errorf("UpdatedAt not UTC: location=%v", got.UpdatedAt.Location())
			}

			// For the 100-block case we only verify count and order, not every entry.
			if tc.name == "valid with 100 blocks (maximum)" {
				if len(got.Blocks) != 100 {
					t.Errorf("Blocks len: got %d, want 100", len(got.Blocks))
				}
				for i := 1; i < len(got.Blocks); i++ {
					if got.Blocks[i].Code < got.Blocks[i-1].Code {
						t.Errorf("Blocks not sorted at index %d", i)
					}
				}
				return
			}

			if len(got.Blocks) != len(tc.want.Blocks) {
				t.Fatalf("Blocks len: got %d, want %d", len(got.Blocks), len(tc.want.Blocks))
			}
			for i, b := range got.Blocks {
				wantB := tc.want.Blocks[i]
				if b.Code != wantB.Code {
					t.Errorf("Blocks[%d].Code: got %d, want %d", i, b.Code, wantB.Code)
				}
				// Normalize json.Number for comparison (e.g. "200" vs "200").
				if json.Number(b.Value) != json.Number(wantB.Value) {
					t.Errorf("Blocks[%d].Value: got %q, want %q", i, b.Value, wantB.Value)
				}
			}
		})
	}
}

func TestBlockMessageIsNewerThan(t *testing.T) {
	base := mustTime("2024-01-15T10:30:00Z")

	msg := &BlockMessage{UpdatedAt: base}

	tests := []struct {
		name   string
		stored time.Time
		want   bool
	}{
		{
			name:   "newer than stored",
			stored: base.Add(-time.Second),
			want:   true,
		},
		{
			name:   "equal to stored",
			stored: base,
			want:   false,
		},
		{
			name:   "older than stored",
			stored: base.Add(time.Second),
			want:   false,
		},
		{
			name:   "zero stored (item does not exist)",
			stored: time.Time{},
			want:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := msg.IsNewerThan(tc.stored); got != tc.want {
				t.Errorf("IsNewerThan(%v) = %v, want %v", tc.stored, got, tc.want)
			}
		})
	}
}
