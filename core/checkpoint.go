package core

import (
	"errors"
	"sort"
	"time"
)

// Checkpoints (SPEC.md 6.8). A bean.checkpoint names records by id. It makes
// two things detectable from the files alone: a covered record that was
// removed, and a claim that a retired key wrote after the fact. Each one lists
// only ids that no checkpoint covers yet, and the ids include older
// checkpoints, so removing an old one is detected too. Removing the newest
// checkpoint together with what it covers is not: only another copy or an
// outside witness can show that.

const (
	// maxCovers keeps a checkpoint under MaxRecordSize. An id costs 74 bytes.
	maxCovers = 12000
	// A person's write adds a checkpoint when this many records are uncovered,
	// or when the oldest uncovered one is older than autoCheckpointAge.
	autoCheckpointCount = 256
	autoCheckpointAge   = 24 * time.Hour
)

// uncovered lists the records a new checkpoint should name: valid, from the
// person or a paired agent, attested, and not covered yet.
func (l *Loaded) uncovered() []*Record {
	labels := l.AgentLabels()
	var out []*Record
	for id, r := range l.Report.Records {
		if !l.Report.Valid[id] || l.Report.Unattested[id] || l.Report.Covered(l.Me, id) {
			continue
		}
		if r.Author == l.Me || labels[r.Author] != "" {
			out = append(out, r)
		}
	}
	return out
}

// Checkpoint writes checkpoints over every uncovered record, and over the
// unattested claims named in accept, which the person vouches for this way.
// It returns the new ids; none when there is nothing new to cover.
func (s *Store) Checkpoint(accept []string) ([]string, error) {
	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	l, err := s.Load()
	if err != nil {
		return nil, err
	}
	return s.checkpointWith(l, accept)
}

func (s *Store) checkpointWith(l *Loaded, accept []string) ([]string, error) {
	w, err := s.writerFor(l, false)
	if err != nil {
		return nil, err
	}
	var ids []string
	work := false
	for _, r := range l.uncovered() {
		ids = append(ids, r.ID)
		work = work || !r.IsCheckpoint() // a lone checkpoint must not make another
	}
	for _, x := range accept {
		r, err := l.Resolve(x)
		if err != nil {
			return nil, err
		}
		if !l.Report.Unattested[r.ID] {
			return nil, errors.New("input missing: " + x + " is not an unattested claim")
		}
		ids = append(ids, r.ID)
		work = true
	}
	if !work {
		return nil, nil
	}
	sort.Strings(ids)
	var out []string
	prev := ""
	for len(ids) > 0 {
		n := min(len(ids), maxCovers)
		chunk := append([]string{}, ids[:n]...)
		ids = ids[n:]
		if prev != "" {
			chunk = append(chunk, prev) // so no part of a large checkpoint goes unnoticed
			sort.Strings(chunk)
		}
		covers := make([]any, len(chunk))
		for i, x := range chunk {
			covers[i] = x
		}
		c := map[string]any{
			"v": 0.0, "nonce": NewNonce(), "type": "bean.checkpoint",
			"subject": w.author, "author": w.author, "kid": w.signer.Public(),
			"value":  map[string]any{"covers": covers},
			"times":  map[string]any{"observed": nil, "imported": s.nowString()},
			"source": map[string]any{"system": "self", "adapter": Adapter, "ref": nil},
		}
		id, data, err := SignRecord(c, w.signer)
		if err != nil {
			return out, err
		}
		if fr := CheckFile(PathOf(id), data); fr.Err != nil {
			return out, fr.Err
		}
		if _, err := writeIfAbsent(s.path(PathOf(id)), data, 0o600); err != nil {
			return out, err
		}
		out = append(out, id)
		prev = id
	}
	return out, nil
}

// autoCheckpoint runs after a person's write. l is the load from before the
// write, and added is how many records the write added. The caller holds the
// lock. It costs a second load, so it runs only when one is due.
func (s *Store) autoCheckpoint(l *Loaded, added int) error {
	if added == 0 || l.Local == nil {
		return nil
	}
	cutoff := s.Now().UTC().Add(-autoCheckpointAge).Format("2006-01-02T15:04:05Z")
	n := added
	due := false
	for _, r := range l.uncovered() {
		if r.IsCheckpoint() {
			continue
		}
		n++
		due = due || r.Imported <= cutoff
	}
	if n < autoCheckpointCount && !due {
		return nil
	}
	l2, err := s.Load()
	if err != nil {
		return err
	}
	_, err = s.checkpointWith(l2, nil)
	return err
}
