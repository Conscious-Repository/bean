package core

import "errors"

// BlobIDOf returns the id that PutBlob would give plaintext, and writes
// nothing. Blob encryption is deterministic (SIV), so the id is known before
// the file is stored. bean import uses it to check an importer's records
// before anything is written.
func (s *Store) BlobIDOf(plaintext []byte) (string, error) {
	secret, err := s.identitySecret()
	if err != nil {
		return "", err
	}
	out, err := EncryptBlob(BlobKey(secret), plaintext)
	if err != nil {
		return "", err
	}
	return HashID(out), nil
}

// InputPurged reports whether the person deleted the record that a partial
// record line (SPEC.md 9.2) stands for: its ref is in local/purged-refs.jsonl.
// It computes the ref as Append does, with the person's identity as the
// default subject. A line without source.ref_input has no ref and is never
// purged.
func (s *Store) InputPurged(line []byte) (bool, error) {
	v, err := Parse(line)
	obj, ok := v.(map[string]any)
	if err != nil || !ok {
		return false, errors.New("bad json")
	}
	src, _ := obj["source"].(map[string]any)
	if src == nil {
		return false, errors.New("bad field: source")
	}
	raw, has := src["ref_input"]
	if !has {
		return false, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return false, errors.New("bad field: source.ref_input")
	}
	secret, err := s.identitySecret()
	if err != nil {
		return false, err
	}
	subject, has := obj["subject"].(string)
	if _, set := obj["subject"]; set && !has {
		return false, errors.New("bad field: subject")
	}
	if !has {
		if subject, err = s.Identity(); err != nil {
			return false, err
		}
	}
	system, _ := src["system"].(string)
	purged, err := s.readPurgedRefs()
	if err != nil {
		return false, err
	}
	return purged[refLine(subject, system, DedupeRef(DedupeKey(secret), arr))], nil
}
