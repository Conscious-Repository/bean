package core

import "errors"

// CheckPartial reports whether Append would refuse a partial record line
// (SPEC.md 9.2) on its checks of the record itself, and writes nothing. It
// completes a copy as Append does, signs it in memory with the person's key,
// and runs the file checks (SPEC.md 3.8) and the write checks on it. It does
// not dedupe, and it ignores source.supersedes_ref. bean import calls it on
// the record that keeps a file before it stores the file: a record Append
// would refuse must not leave the file stored with no record naming it.
func (s *Store) CheckPartial(line []byte) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	l, err := s.load(false)
	if err != nil {
		return err
	}
	w, err := s.writerFor(l, false)
	if err != nil {
		return err
	}
	if err := l.canWrite(w); err != nil {
		return err
	}
	v, err := Parse(line)
	obj, ok := v.(map[string]any)
	if err != nil || !ok {
		return errors.New("bad json")
	}
	for _, k := range forbiddenPartial {
		if _, has := obj[k]; has {
			return badField(k)
		}
	}
	obj["v"] = 0.0
	obj["nonce"] = NewNonce()
	obj["author"] = w.author
	obj["kid"] = w.signer.Public()
	if _, has := obj["subject"]; !has {
		obj["subject"] = w.identity
	}
	times, _ := obj["times"].(map[string]any)
	if times == nil {
		times = map[string]any{"observed": nil}
		obj["times"] = times
	}
	times["imported"] = s.nowString()
	src, _ := obj["source"].(map[string]any)
	if src == nil {
		return badField("source")
	}
	var key []byte
	if w.secret != nil {
		key = DedupeKey(w.secret)
	}
	ref, hasInput, err := inputRef(src, "ref_input", key, 1)
	if err != nil {
		return err
	}
	if hasInput {
		src["ref"] = ref
	}
	if _, has := src["ref"]; !has {
		src["ref"] = nil
	}
	if _, _, err := inputRef(src, "supersedes_ref", key, 1); err != nil {
		return err
	}
	id, data, err := SignRecord(obj, w.signer)
	if err != nil {
		return err
	}
	fr := CheckFile(PathOf(id), data)
	if fr.Err != nil {
		return fr.Err
	}
	return l.canWriteRecord(fr.Record)
}

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
// It computes the ref as Append does, from source.ref_input, else it takes a
// ready-made source.ref, with the person's identity as the default subject.
// A line with neither has no ref and is never purged.
//
// The system does not count. A file deleted under one -system is deleted
// under every one: the line's ref_input is computed again with each system
// that the purged refs name, and a ready-made ref matches under any system.
// Only bean import calls this, for the record that keeps a file.
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
	var arr []any
	ref := ""
	if raw, has := src["ref_input"]; has {
		if arr, ok = raw.([]any); !ok {
			return false, errors.New("bad field: source.ref_input")
		}
	} else if raw, has := src["ref"]; has && raw != nil {
		if ref, ok = raw.(string); !ok || !IsRef(ref) {
			return false, errors.New("bad field: source.ref")
		}
	} else {
		return false, nil
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
	// The systems the purged refs name, and the refs under each.
	systems := map[string]bool{system: true}
	refs := map[string]bool{}
	for l := range purged {
		x, err := Parse([]byte(l))
		t, _ := x.([]any)
		if err != nil || len(t) != 3 {
			continue
		}
		if who, _ := t[0].(string); who != subject {
			continue
		}
		sys, _ := t[1].(string)
		r, _ := t[2].(string)
		systems[sys] = true
		refs[r] = true
	}
	if arr == nil {
		return refs[ref], nil
	}
	key := DedupeKey(secret)
	for sys := range systems {
		in := arr
		// RefInputNative and RefInputTuple put the system third.
		if len(arr) > 2 && arr[0] == "bean/v0/ref" && arr[2] == system {
			in = append(append([]any{}, arr[:2]...), sys)
			in = append(in, arr[3:]...)
		}
		if purged[refLine(subject, sys, DedupeRef(key, in))] {
			return true, nil
		}
	}
	return false, nil
}
