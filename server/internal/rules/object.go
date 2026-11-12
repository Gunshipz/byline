package rules

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// KV is one key/value pair of a JSON object, with the value kept as raw,
// undecoded JSON.
type KV struct {
	Key   string
	Value json.RawMessage
}

// Object is a JSON object that keeps the key order its source text had,
// instead of the arbitrary order a Go map gives.
type Object []KV

// UnmarshalJSON decodes one level of a JSON object into o, keeping key
// order and leaving each value as raw JSON for the caller to decode further.
func (o *Object) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("rules: expected a JSON object, got %v", tok)
	}
	var out Object
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("rules: expected an object key, got %v", keyTok)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}
		out = append(out, KV{Key: key, Value: raw})
	}
	if _, err := dec.Token(); err != nil { // closing }
		return err
	}
	*o = out
	return nil
}

// MarshalJSON writes o back out in the key order it was decoded with (or
// built in): "{", then each "key":compact(value) joined by ",", then "}".
// An empty Object marshals as "{}".
func (o Object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, kv := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(kv.Key)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		value := kv.Value
		if len(value) == 0 {
			value = json.RawMessage("null")
		}
		if err := json.Compact(&buf, value); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// DecodeObject decodes raw (expected to hold a JSON object) into an Object,
// keeping key order.
func DecodeObject(raw json.RawMessage) (Object, error) {
	var o Object
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, err
	}
	return o, nil
}
