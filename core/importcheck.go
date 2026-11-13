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
	_, _, err := s.SignPartial(line)
	return err
}

// CheckImported is CheckPartial for a line of an importer that bean ran:
// it also holds the line to what an importer may write (AppendImported).
func (s *Store) CheckImported(line []byte) error {
	_, _, err := s.signPartial(line, true)
	return err
}

// SignPartial completes and signs one partial record line with the person's
// key, as Append would, checks it as CheckPartial does, and writes nothing.
// It returns the record's id and its bytes. AddSigned stores those bytes
// later. bean share uses the pair: the share record goes into the bundle
// first, and into the log only once the bundle is written and checked, so a
// failure leaves no record of a disclosure that did not happen.
func (s *Store) SignPartial(line []byte) (string, []byte, error) {
	return s.signPartial(line, false)
}

func (s *Store) signPartial(line []byte, importer bool) (string, []byte, error) {
	unlock, err := s.lock()
	if err != nil {
		return "", nil, err
	}
	defer unlock()
	l, err := s.load(false)
	if err != nil {
		return "", nil, err
	}
	w, err := s.writerFor(l, false)
	if err != nil {
		return "", nil, err
	}
	if err := l.canWrite(w); err != nil {
		return "", nil, err
	}
	w.importer = importer
	var key []byte
	if w.secret != nil {
		key = DedupeKey(w.secret)
	}
	obj, src, err := s.completePartial(w, line, key, 1)
	if err != nil {
		return "", nil, err
	}
	if _, _, err := inputRef(src, "supersedes_ref", key, 1); err != nil {
		return "", nil, err
	}
	id, data, err := SignRecord(obj, w.signer)
	if err != nil {
		return "", nil, err
	}
	fr := CheckFile(PathOf(id), data)
	if fr.Err != nil {
		return "", nil, fr.Err
	}
	return id, data, l.canWriteRecord(fr.Record)
}

// AddSigned stores a record that SignPartial signed: the same bytes, under
// the same id. It takes only a record that the person's current key signed
// as the person, and runs the write checks again, since the log may have
// changed in between.
func (s *Store) AddSigned(id string, data []byte) error {
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
	fr := CheckFile(PathOf(id), data)
	if fr.Err != nil {
		return fr.Err
	}
	if fr.Record.Author != w.author || fr.Record.Kid != w.signer.Public() {
		return errors.New("not a record that this log's key signed")
	}
	if err := l.canWriteRecord(fr.Record); err != nil {
		return err
	}
	_, err = writeIfAbsent(s.path(PathOf(id)), data, 0o600)
	return err
}

// AddFromKey stores a record that another person's key signed about the
// local person: a note of a peer (docs/SHARE.md, "A signed reply"). The
// caller has checked what the record says. This checks what such a record
// may be at all: signed by a bare key, about the local person, and with no
// power over any other record. It is not a correction, not a deletion, not
// an identity record or one of bean's own, and it keeps no file.
func (s *Store) AddFromKey(id string, data []byte) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	l, err := s.load(false)
	if err != nil {
		return err
	}
	if l.ID == "" || l.Local == nil {
		return ErrNotInitialized
	}
	if l.IsTooNew() || l.IsFrozen() {
		return errors.New("this log takes no record now: it is frozen, or has records from a newer bean")
	}
	fr := CheckFile(PathOf(id), data)
	if fr.Err != nil {
		return fr.Err
	}
	r := fr.Record
	switch {
	case !IsTaggedKey(r.Author) || r.Kid != r.Author:
		return errors.New("not a record that a key signed by itself")
	case r.Subject != l.ID:
		return errors.New("not a record about this log's person")
	case r.Author == l.Local.Signer.Public():
		return errors.New("a record that this log's own key signed is not another person's")
	case r.IsIdentity() || IsCoreType(r.Type) || r.Type == "agent.pair":
		return errors.New("bad type: another person's key adds no record of that type")
	case len(r.Targets("supersedes")) > 0 || len(r.Targets("purges")) > 0 || len(r.Targets("succeeds")) > 0 || len(r.Targets("seals")) > 0:
		return errors.New("another person's key corrects and deletes nothing")
	case len(r.Blobs) > 0:
		return errors.New("another person's record keeps no file")
	case l.DeletedByPerson(id):
		return errors.New("the person deleted this record: it is not taken again")
	}
	if err := l.canWriteRecord(r); err != nil {
		return err
	}
	_, err = writeIfAbsent(s.path(PathOf(id)), data, 0o600)
	return err
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
