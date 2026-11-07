package core

import (
	"bufio"
	"errors"
	"sort"
	"strings"
)

// MigrateResult is the output of Migrate.
type MigrateResult struct {
	Identity     string
	Words        []string
	Records      int
	Blobs        int
	MissingBlobs int
	Skipped      int // too large to carry the migration context
	Unattested   int // claims of a retired key, left out
}

// Migrate starts a new identity in the empty folder dst from what this copy
// shows as current and trusted. A frozen identity has no way back (SPEC.md
// 5.7), so this is the way forward. The new records are signed by the new key
// and keep the old id in context.migrated_from. Nothing links the two
// identities by signature: whoever reads the new log decides to trust it.
//
// A dedupe ref keeps its old value, but the new identity makes different ones,
// so a later import of an old export adds its records again. Agents must be
// set up again, because their copies still write about the old identity, so a
// pairing does not move. A claim of a retired key does not move: signing it
// with the new key would vouch for what may be a thief's record. Its count is
// in the result.
func (s *Store) Migrate(dst *Store, key NewKey) (MigrateResult, error) {
	var res MigrateResult
	unlock, err := s.lock()
	if err != nil {
		return res, err
	}
	defer unlock()
	l, err := s.Load()
	if err != nil {
		return res, err
	}
	if l.Local == nil {
		return res, ErrNotInitialized
	}
	if l.IsTooNew() {
		return res, l.Standing()
	}
	if entries, err := dst.listObjectsEmpty(); err != nil {
		return res, err
	} else if !entries {
		return res, errors.New("migrate folder not empty")
	}
	if res.Identity, res.Words, err = dst.Init(key); err != nil {
		return res, err
	}
	dl, err := dst.Load()
	if err != nil {
		return res, err
	}
	w, err := dst.writerFor(dl, false)
	if err != nil {
		return res, err
	}
	labels := l.AgentLabels()
	moved := map[string]string{} // old id -> new id
	skipped := map[string]bool{}
	var move func(id string) error
	move = func(id string) error {
		if _, done := moved[id]; done || skipped[id] {
			return nil
		}
		r := l.Report.Records[id]
		obj := map[string]any{}
		for k, v := range r.Raw {
			switch k {
			case "v", "nonce", "author", "kid", "id", "sig", "subject", "refs", "blobs":
			default:
				obj[k] = v
			}
		}
		ctx := map[string]any{}
		for k, v := range r.Context {
			ctx[k] = v
		}
		ctx["migrated_from"] = id
		ctx["migrated_imported"] = r.Imported
		if r.Author != l.Me {
			ctx["migrated_author"] = r.Author
			// All records now have one author, so an agent's copy of a ref
			// would make the person's record its duplicate.
			src := map[string]any{}
			for k, v := range r.Raw["source"].(map[string]any) {
				src[k] = v
			}
			src["ref"] = nil
			obj["source"] = src
		}
		var refs []any
		for _, ref := range r.Refs {
			if ref.Rel != "about" || !l.Current(ref.ID) || !l.Trusted(l.Report.Records[ref.ID], labels) {
				continue // the current value replaces what it corrected
			}
			if err := move(ref.ID); err != nil {
				return err
			}
			if skipped[ref.ID] {
				continue
			}
			refs = append(refs, map[string]any{"rel": "about", "id": moved[ref.ID]})
		}
		if refs != nil {
			obj["refs"] = refs
		}
		value, _ := obj["value"].(map[string]any)
		var blobs []any
		newHash := map[string]string{}
		for _, b := range r.Blobs {
			nh, err := s.moveBlob(dst, b.Hash, newHash)
			if err != nil {
				res.MissingBlobs++
				ctx["migrated_missing_blob"] = b.Hash
				continue
			}
			res.Blobs++
			blobs = append(blobs, map[string]any{"hash": nh, "role": b.Role, "media_type": b.MediaType})
		}
		if blobs != nil {
			obj["blobs"] = blobs
		}
		if old, ok := value["blob"].(string); ok {
			nv := map[string]any{}
			for k, v := range value {
				nv[k] = v
			}
			if nh, ok := newHash[old]; ok {
				nv["blob"] = nh
			} else {
				delete(nv, "blob") // the file is gone; the record must not name it
			}
			obj["value"] = nv
		}
		obj["context"] = ctx
		obj["times"] = cloneTimes(r)
		out, err := dst.appendWith(dl, w, strings.NewReader(string(Canonical(obj))+"\n"))
		if err != nil && (errors.Is(err, bufio.ErrTooLong) || strings.HasPrefix(err.Error(), ErrTooLarge.Error())) {
			// The record is close to the limit, and the context pushed it over.
			skipped[id] = true
			res.Skipped++
			return nil
		}
		if err != nil {
			return err
		}
		if len(out.IDs) != 1 {
			return errors.New("not added: " + id)
		}
		moved[id] = out.IDs[0]
		res.Records++
		return nil
	}
	var ids []string
	for id, r := range l.Report.Records {
		if r.Subject == l.Me && r.Author == l.Me && l.Report.Unattested[id] {
			res.Unattested++
		}
		if l.Current(id) && l.Trusted(r, labels) && !isSetup(r.Type) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := move(id); err != nil {
			return res, err
		}
	}
	_, err = dst.Checkpoint(nil)
	return res, err
}

// moveBlob decrypts a blob here and stores it under dst's key.
func (s *Store) moveBlob(dst *Store, hash string, seen map[string]string) (string, error) {
	pt, err := s.GetBlob(hash)
	if err != nil {
		return "", err
	}
	nh, err := dst.PutBlob(pt)
	if err == nil {
		seen[hash] = nh
	}
	return nh, err
}

func cloneTimes(r *Record) map[string]any {
	return map[string]any{"observed": r.Observed}
}

// listObjectsEmpty reports whether the folder holds no records and no keys.
func (s *Store) listObjectsEmpty() (bool, error) {
	disk, _, err := s.listObjects()
	if err != nil {
		return false, err
	}
	if len(disk) > 0 {
		return false, nil
	}
	_, err = s.Identity()
	return errors.Is(err, ErrNotInitialized), nil
}
