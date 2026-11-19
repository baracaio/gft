package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"uuid"
)

const StoredTimestampFormat = "2006-01-02T15:04:05Z"

var timestampFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
}

type (
	clientRaw struct {
		Code      *string `json:"codigo_cliente"`
		Branch    *int    `json:"numero_agencia"`
		Account   *int    `json:"numero_conta"`
		Dac       *int    `json:"numero_dac"`
		UpdatedAt *string `json:"data_atualizacao"`
	}

	ClientMessage struct {
		Code      uuid.UUID
		Branch    int
		Account   int
		Dac       int
		UpdatedAt time.Time
	}
)

func ParseClientMessage(body string) (*ClientMessage, error) {
	var raw clientRaw

	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return nil, &PermanentError{
				Field:   typeErr.Field,
				Message: fmt.Sprintf("wrong type: expected %s, got JSON %s", typeErr.Type, typeErr.Value),
			}
		}
		return nil, &PermanentError{Message: "invalid JSON: " + err.Error()}
	}

	if raw.Code == nil {
		return nil, &PermanentError{Field: "codigo_cliente", Message: "required field missing"}
	}
	if raw.Branch == nil {
		return nil, &PermanentError{Field: "numero_agencia", Message: "required field missing"}
	}
	if raw.Account == nil {
		return nil, &PermanentError{Field: "numero_conta", Message: "required field missing"}
	}
	if raw.Dac == nil {
		return nil, &PermanentError{Field: "numero_dac", Message: "required field missing"}
	}
	if raw.UpdatedAt == nil {
		return nil, &PermanentError{Field: "data_atualizacao", Message: "required field missing"}
	}

	code, err := uuid.Parse(*raw.Code)
	if err != nil {
		return nil, &PermanentError{
			Field:   "codigo_cliente",
			Message: fmt.Sprintf("invalid GUID format: %q", *raw.Code),
		}
	}

	ts, err := parseTimestamp(*raw.UpdatedAt)
	if err != nil {
		return nil, &PermanentError{Field: "data_atualizacao", Message: err.Error()}
	}

	return &ClientMessage{
		Code:      code,
		Branch:    *raw.Branch,
		Account:   *raw.Account,
		Dac:       *raw.Dac,
		UpdatedAt: ts,
	}, nil
}

func (c *ClientMessage) IsNewerThan(stored time.Time) bool {
	return c.UpdatedAt.After(stored.UTC())
}

func parseTimestamp(s string) (time.Time, error) {
	for _, layout := range timestampFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf(
		"invalid or timezone-less timestamp %q; expected ISO 8601 UTC, e.g. %q",
		s, "2024-01-15T10:30:00Z",
	)
}
