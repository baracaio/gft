package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"uuid"
)

const maxBlocks = 100

type (
	Block struct {
		Code  int
		Value json.Number
	}

	BlockMessage struct {
		Code      uuid.UUID
		Blocks    []Block
		UpdatedAt time.Time
	}
)

func ParseBlockMessage(body string) (*BlockMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return nil, &PermanentError{Message: "invalid JSON: " + err.Error()}
	}

	code, err := extractGUID(raw)
	if err != nil {
		return nil, err
	}

	ts, err := extractTimestamp(raw)
	if err != nil {
		return nil, err
	}

	blocks, err := extractBlocks(raw)
	if err != nil {
		return nil, err
	}

	return &BlockMessage{
		Code:      code,
		Blocks:    blocks,
		UpdatedAt: ts,
	}, nil
}

func (b *BlockMessage) IsNewerThan(stored time.Time) bool {
	return b.UpdatedAt.After(stored.UTC())
}

func extractGUID(raw map[string]json.RawMessage) (uuid.UUID, error) {
	v, ok := raw["codigo_cliente"]
	if !ok {
		return uuid.UUID{}, &PermanentError{Field: "codigo_cliente", Message: "required field missing"}
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return uuid.UUID{}, &PermanentError{Field: "codigo_cliente", Message: "wrong type: expected string"}
	}
	code, err := uuid.Parse(s)
	if err != nil {
		return uuid.UUID{}, &PermanentError{
			Field:   "codigo_cliente",
			Message: fmt.Sprintf("invalid GUID format: %q", s),
		}
	}
	return code, nil
}

func extractTimestamp(raw map[string]json.RawMessage) (time.Time, error) {
	v, ok := raw["data_atualizacao"]
	if !ok {
		return time.Time{}, &PermanentError{Field: "data_atualizacao", Message: "required field missing"}
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return time.Time{}, &PermanentError{Field: "data_atualizacao", Message: "wrong type: expected string"}
	}
	ts, err := parseTimestamp(s)
	if err != nil {
		return time.Time{}, &PermanentError{Field: "data_atualizacao", Message: err.Error()}
	}
	return ts, nil
}

func extractBlocks(raw map[string]json.RawMessage) ([]Block, error) {
	codeList := make(map[int]struct{})
	valueList := make(map[int]struct{})

	for key := range raw {
		if s, ok := trimPrefix(key, "codigo_bloqueio_"); ok {
			n, err := strconv.Atoi(s)
			if err == nil {
				codeList[n] = struct{}{}
			}
		}
		if s, ok := trimPrefix(key, "valor_bloqueio_"); ok {
			n, err := strconv.Atoi(s)
			if err == nil {
				valueList[n] = struct{}{}
			}
		}
	}

	// Incomplete pair: codigo without valor
	for n := range codeList {
		if _, ok := valueList[n]; !ok {
			return nil, &PermanentError{
				Field:   fmt.Sprintf("valor_bloqueio_%d", n),
				Message: fmt.Sprintf("incomplete pair: codigo_bloqueio_%d has no matching valor_bloqueio_%d", n, n),
			}
		}
	}
	// Incomplete pair: valor without codigo
	for n := range valueList {
		if _, ok := codeList[n]; !ok {
			return nil, &PermanentError{
				Field:   fmt.Sprintf("codigo_bloqueio_%d", n),
				Message: fmt.Sprintf("incomplete pair: valor_bloqueio_%d has no matching codigo_bloqueio_%d", n, n),
			}
		}
	}

	if len(codeList) > maxBlocks {
		return nil, &PermanentError{
			Message: fmt.Sprintf("too many blocks: %d exceeds maximum of %d", len(codeList), maxBlocks),
		}
	}

	seen := make(map[int]struct{}, len(codeList))
	blocks := make([]Block, 0, len(codeList))

	for n := range codeList {
		var code int
		if err := json.Unmarshal(raw[fmt.Sprintf("codigo_bloqueio_%d", n)], &code); err != nil {
			return nil, &PermanentError{
				Field:   fmt.Sprintf("codigo_bloqueio_%d", n),
				Message: "wrong type: expected integer",
			}
		}

		var value json.Number
		if err := json.Unmarshal(raw[fmt.Sprintf("valor_bloqueio_%d", n)], &value); err != nil {
			return nil, &PermanentError{
				Field:   fmt.Sprintf("valor_bloqueio_%d", n),
				Message: "wrong type: expected number",
			}
		}
		if value == "" {
			return nil, &PermanentError{
				Field:   fmt.Sprintf("valor_bloqueio_%d", n),
				Message: "wrong type: expected number",
			}
		}

		if _, dup := seen[code]; dup {
			return nil, &PermanentError{
				Field:   fmt.Sprintf("codigo_bloqueio_%d", n),
				Message: fmt.Sprintf("duplicate block code: %d", code),
			}
		}
		seen[code] = struct{}{}

		blocks = append(blocks, Block{Code: code, Value: value})
	}

	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Code < blocks[j].Code
	})

	return blocks, nil
}

func trimPrefix(s, prefix string) (string, bool) {
	if strings.HasPrefix(s, prefix) {
		return strings.TrimPrefix(s, prefix), true
	}
	return "", false
}
