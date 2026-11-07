package core

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

// MigrateResult is the output of Migrate.
type MigrateResult struct {
	Identity     string
	Words        []string
	Records      int
	Blobs        int
	MissingBlobs int
	Disputed     int // moved with the old value: only one side of the fork corrected them
	LeftOut      int // not moved: by other authors, or in doubt after the freeze
	TooLarge     int // too close to the size limit to carry the migration context
}

// Migrate starts a new identity in an empty folder from a frozen one. A frozen
// identity has no way back (SPEC.md 5.7), so this is the way forward. On any
// other log, `bean rotate` is the answer.
//
// Signing a record with the new key says that the person stands behind it, so
// only the person's own records that this copy shows move: not an agent's, a
// stranger's, or a claim in doubt. A claim in doubt can be the person's, for
// example what they logged after the fork, or a thief's: a key is not a machine.
// So it moves only if the person names it in also. What stays behind stays in
// the old log, and the result counts it. A record that one side of the fork corrected moves with its old
// value, and context.migrated_disputed_by names the correction: bean cannot
// tell which side is right. Nothing links the two identities by signature:
// after a fork two parties hold the seed.
//
// A dedupe ref keeps its old value, but the new identity makes different ones,
// so a later import of an old export adds its records again.
func (s *Store) Migrate(dst *Store, key NewKey, also ...string) (MigrateResult, error) {
	var res MigrateResult
	unlock, err := s.lock()
	if err != nil {
		return res, err
	}
	defer unlock()
	l, err := s.load(false)
	if err != nil {
		return res, err
	}
	if l, err = l.migratable(s); err != nil {
		return res, err
	}
	named := map[string]bool{}
	for _, x := range also {
		r, err := l.Resolve(x)
		if err != nil {
			return res, err
		}
		if !l.Report.InDoubt(r.ID) || r.Author != l.ID || r.Subject != l.ID || r.IsDestructive() || IsCoreType(r.Type) || isSetup(r.Type) {
			return res, errors.New("input missing: " + x + " is not a claim of yours in doubt")
		}
		named[r.ID] = true
	}
	if entries, err := os.ReadDir(dst.Dir); err == nil && len(entries) > 0 {
		return res, errors.New("migrate folder not empty")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return res, err
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
	movable := func(id string) bool { return l.movable(id) || named[id] }
	refs := map[string]bool{}    // no two moved records with one ref: one would hide the other
	moved := map[string]string{} // old id -> new id
	skipped := map[string]bool{}
	var move func(id string) error
	move = func(id string) error {
		if _, done := moved[id]; done || skipped[id] {
			return nil
		}
		r := l.Report.Records[id]
		if r.Source.Ref != "" {
			key := r.Source.System + "\x00" + r.Source.Ref
			if refs[key] {
				skipped[id] = true
				res.LeftOut++
				return nil
			}
			refs[key] = true
		}
		obj := map[string]any{}
		for k, v := range r.Raw {
			switch k {
			case "v", "nonce", "author", "kid", "id", "sig", "subject", "refs", "blobs", "times":
			default:
				obj[k] = v
			}
		}
		obj["times"] = map[string]any{"observed": r.Observed}
		// A stored source can hold keys that append reads as instructions.
		src := map[string]any{}
		for k, v := range r.Raw["source"].(map[string]any) {
			if k != "ref_input" && k != "supersedes_ref" {
				src[k] = v
			}
		}
		obj["source"] = src
		ctx := map[string]any{}
		for k, v := range r.Context {
			ctx[k] = v
		}
		ctx["migrated_from"] = id
		ctx["migrated_imported"] = r.Imported
		if d := l.Report.DisputedBy[id]; len(d) > 0 {
			list := make([]any, len(d))
			for i, x := range d {
				list[i] = x
			}
			ctx["migrated_disputed_by"] = list
			res.Disputed++
		}
		var about []any
		var lost []any
		for _, ref := range r.Refs {
			if ref.Rel != "about" {
				continue // the current value replaces what it corrected
			}
			if !movable(ref.ID) {
				lost = append(lost, ref.ID)
				continue
			}
			if err := move(ref.ID); err != nil {
				return err
			}
			if skipped[ref.ID] {
				lost = append(lost, ref.ID)
				continue
			}
			about = append(about, map[string]any{"rel": "about", "id": moved[ref.ID]})
		}
		if about != nil {
			obj["refs"] = about
		}
		if lost != nil {
			ctx["migrated_about"] = lost // a link that the new log cannot hold
		}
		var blobs []any
		newHash := map[string]string{}
		for _, b := range r.Blobs {
			pt, err := s.GetBlob(b.Hash)
			if err != nil {
				res.MissingBlobs++
				ctx["migrated_missing_blob"] = b.Hash
				continue
			}
			nh, err := dst.PutBlob(pt)
			if err != nil {
				return err
			}
			newHash[b.Hash] = nh
			res.Blobs++
			blobs = append(blobs, map[string]any{"hash": nh, "role": b.Role, "media_type": b.MediaType})
		}
		if blobs != nil {
			obj["blobs"] = blobs
		}
		if value, _ := obj["value"].(map[string]any); value != nil {
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
		}
		obj["context"] = ctx
		out, err := dst.appendWith(dl, w, strings.NewReader(string(Canonical(obj))+"\n"))
		if err != nil && (errors.Is(err, bufio.ErrTooLong) || strings.HasPrefix(err.Error(), ErrTooLarge.Error())) {
			skipped[id] = true // the context pushed it over the limit
			res.TooLarge++
			return nil
		}
		if err != nil {
			return err
		}
		if len(out.IDs) != 1 {
			skipped[id] = true
			res.LeftOut++
			return nil
		}
		moved[id] = out.IDs[0]
		res.Records++
		return nil
	}
	// What this copy shows goes first, so a claim the person also named cannot
	// take the place of a real reading through a copied ref.
	for _, id := range sortedKeys(l.Report.Records) {
		if l.movable(id) {
			if err := move(id); err != nil {
				return res, err
			}
		}
	}
	for _, id := range sortedKeys(named) {
		if err := move(id); err != nil {
			return res, err
		}
	}
	for _, r := range l.leftBehind() {
		if !named[r.ID] {
			res.LeftOut++
		}
	}
	return res, nil
}

// migratable returns l judged so that migrate can run, or the reason it cannot.
func (l *Loaded) migratable(s *Store) (*Loaded, error) {
	if l.ID == "" {
		return nil, ErrNotInitialized
	}
	if _, err := s.identitySecret(); err != nil {
		return nil, err // the 24 words give this copy the secret (bean restore)
	}
	if l.IsTooNew() {
		// A record of a newer format by a key at the fork must not close the
		// only way forward, so judge the log without it.
		if len(l.Report.HeadHalt[l.ID]) == 0 || !contains(l.past(l.ID).Report.Compromised, l.ID) {
			return nil, l.TooNewError()
		}
		l = l.past(l.ID)
	}
	if !l.IsFrozen() {
		return nil, errors.New("not frozen: this log can go on. To change a key, run bean rotate")
	}
	return l, nil
}

// movable reports a record that moves without the person naming it.
func (l *Loaded) movable(id string) bool {
	r := l.Report.Records[id]
	return r != nil && r.Subject == l.ID && r.Author == l.ID && !isSetup(r.Type) && l.Current(id)
}

// leftBehind lists what a migration does not move by itself, in id order.
func (l *Loaded) leftBehind() []*Record {
	var out []*Record
	for _, id := range sortedKeys(l.Report.Records) {
		r := l.Report.Records[id]
		if (r.Subject == l.ID || r.Author == l.ID) && !IsCoreType(r.Type) && !isSetup(r.Type) && (l.Current(id) || l.Report.InDoubt(id)) && !l.movable(id) {
			out = append(out, r)
		}
	}
	return out
}

// MigrateList returns what a migration would leave behind, so the person can
// name the records in doubt that are theirs.
func (s *Store) MigrateList() ([]*Record, *Loaded, error) {
	l, err := s.Load()
	if err != nil {
		return nil, nil, err
	}
	if l, err = l.migratable(s); err != nil {
		return nil, nil, err
	}
	return l.leftBehind(), l, nil
}
