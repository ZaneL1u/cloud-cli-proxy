package repository

import (
	"encoding/json"
	"fmt"
)

// jsonText scans SQLite TEXT/NULL JSON columns into bytes before callers decode
// them into typed fields. database/sql cannot scan a driver string directly into
// json.RawMessage because RawMessage is a named []byte type.
type jsonText []byte

func (j *jsonText) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		*j = nil
		return nil
	case string:
		*j = append((*j)[:0], v...)
		return nil
	case []byte:
		*j = append((*j)[:0], v...)
		return nil
	default:
		return fmt.Errorf("unsupported JSON text source type %T", value)
	}
}

func (j jsonText) RawMessage() json.RawMessage {
	if len(j) == 0 {
		return nil
	}
	out := make([]byte, len(j))
	copy(out, j)
	return json.RawMessage(out)
}
