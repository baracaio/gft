package domain

import (
	"errors"
	"testing"
	"time"
	"uuid"
)

const (
	testGUID = "550e8400-e29b-41d4-a716-446655440000"
	testTS   = "2024-01-15T10:30:00Z"
)

func mustTime(s string) time.Time {
	t, err := time.Parse(StoredTimestampFormat, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseClientMessage(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		want      *ClientMessage
		wantField string
	}{
		{
			name: "valid message",
			body: `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","numero_agencia":10,"numero_conta":20,"numero_dac":3,"data_atualizacao":"2024-01-15T10:30:00Z"}`,
			want: &ClientMessage{
				Code:      uuid.MustParse(testGUID),
				Branch:    10,
				Account:   20,
				Dac:       3,
				UpdatedAt: mustTime(testTS),
			},
		},
		{
			name: "valid with fractional seconds",
			body: `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","numero_agencia":1,"numero_conta":2,"numero_dac":0,"data_atualizacao":"2024-01-15T10:30:00.123Z"}`,
			want: &ClientMessage{
				Code:      uuid.MustParse(testGUID),
				Branch:    1,
				Account:   2,
				Dac:       0,
				UpdatedAt: time.Date(2024, 1, 15, 10, 30, 0, 123_000_000, time.UTC),
			},
		},
		{
			name: "valid with offset timezone normalized to UTC",
			body: `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","numero_agencia":1,"numero_conta":2,"numero_dac":0,"data_atualizacao":"2024-01-15T07:30:00-03:00"}`,
			want: &ClientMessage{
				Code:      uuid.MustParse(testGUID),
				Branch:    1,
				Account:   2,
				Dac:       0,
				UpdatedAt: mustTime("2024-01-15T10:30:00Z"),
			},
		},
		{
			name:      "invalid JSON",
			body:      `{not json}`,
			wantField: "",
		},
		{
			name:      "type mismatch: numero_agencia is string",
			body:      `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","numero_agencia":"abc","numero_conta":2,"numero_dac":0,"data_atualizacao":"2024-01-15T10:30:00Z"}`,
			wantField: "numero_agencia",
		},
		{
			name:      "missing codigo_cliente",
			body:      `{"numero_agencia":1,"numero_conta":2,"numero_dac":0,"data_atualizacao":"2024-01-15T10:30:00Z"}`,
			wantField: "codigo_cliente",
		},
		{
			name:      "missing numero_agencia",
			body:      `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","numero_conta":2,"numero_dac":0,"data_atualizacao":"2024-01-15T10:30:00Z"}`,
			wantField: "numero_agencia",
		},
		{
			name:      "missing numero_conta",
			body:      `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","numero_agencia":1,"numero_dac":0,"data_atualizacao":"2024-01-15T10:30:00Z"}`,
			wantField: "numero_conta",
		},
		{
			name:      "missing numero_dac",
			body:      `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","numero_agencia":1,"numero_conta":2,"data_atualizacao":"2024-01-15T10:30:00Z"}`,
			wantField: "numero_dac",
		},
		{
			name:      "missing data_atualizacao",
			body:      `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","numero_agencia":1,"numero_conta":2,"numero_dac":0}`,
			wantField: "data_atualizacao",
		},
		{
			name:      "invalid GUID",
			body:      `{"codigo_cliente":"not-a-guid","numero_agencia":1,"numero_conta":2,"numero_dac":0,"data_atualizacao":"2024-01-15T10:30:00Z"}`,
			wantField: "codigo_cliente",
		},
		{
			name:      "timezone-less timestamp",
			body:      `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","numero_agencia":1,"numero_conta":2,"numero_dac":0,"data_atualizacao":"2024-01-15T10:30:00"}`,
			wantField: "data_atualizacao",
		},
		{
			name:      "empty timestamp",
			body:      `{"codigo_cliente":"550e8400-e29b-41d4-a716-446655440000","numero_agencia":1,"numero_conta":2,"numero_dac":0,"data_atualizacao":""}`,
			wantField: "data_atualizacao",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseClientMessage(tc.body)

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
			if got.Branch != tc.want.Branch {
				t.Errorf("Branch: got %d, want %d", got.Branch, tc.want.Branch)
			}
			if got.Account != tc.want.Account {
				t.Errorf("Account: got %d, want %d", got.Account, tc.want.Account)
			}
			if got.Dac != tc.want.Dac {
				t.Errorf("Dac: got %d, want %d", got.Dac, tc.want.Dac)
			}
			if !got.UpdatedAt.Equal(tc.want.UpdatedAt) {
				t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, tc.want.UpdatedAt)
			}
			if got.UpdatedAt.Location() != time.UTC {
				t.Errorf("UpdatedAt not UTC: location=%v", got.UpdatedAt.Location())
			}
		})
	}
}

func TestClientMessageIsNewerThan(t *testing.T) {
	base := mustTime("2024-01-15T10:30:00Z")

	msg := &ClientMessage{UpdatedAt: base}

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
