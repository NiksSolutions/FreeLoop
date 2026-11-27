package engine

import (
	"bytes"
	"encoding/json"
	"errors"
)

// mergePatch applies patch, a JSON merge patch (RFC 7386), to doc: an object's members
// merge into the target's, null removes one, and anything else replaces what was
// there, a list as a whole. Numbers keep their digits.
func mergePatch(doc, patch []byte) ([]byte, error) {
	var d, p any
	if err := decodeNumbers(doc, &d); err != nil {
		return nil, err
	}
	if err := decodeNumbers(patch, &p); err != nil {
		return nil, err
	}
	if _, ok := p.(map[string]any); !ok {
		return nil, errors.New("a merge patch is an object")
	}
	return json.Marshal(mergeValue(d, p))
}

func mergeValue(target, patch any) any {
	p, ok := patch.(map[string]any)
	if !ok {
		return patch
	}
	t, ok := target.(map[string]any)
	if !ok {
		t = map[string]any{}
	}
	for k, v := range p {
		if v == nil {
			delete(t, k)
			continue
		}
		t[k] = mergeValue(t[k], v)
	}
	return t
}

func decodeNumbers(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode(v)
}
