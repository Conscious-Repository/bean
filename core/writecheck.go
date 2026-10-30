package core

import (
	"errors"
	"fmt"
)

// Fail closed on write (DECISIONS 108). A writer must never get an id back
// for a record that the verifier of this same folder rejects: the person
// would believe the entry is stored when no reader will ever show it.

// canWrite refuses a writer whose records this folder would not accept.
func (l *Loaded) canWrite(w writer) error {
	if IsTaggedKey(w.author) {
		return nil // a bare key has no chain; it is always its own head
	}
	rp := l.Report
	if contains(rp.TooNew, w.author) {
		return errors.New("too new: " + w.author + ": this log has records from a newer bean; update bean before you write")
	}
	if contains(rp.Compromised, w.author) {
		return errors.New("compromised: " + w.author + ": the identity is frozen (SPEC.md 5.7) and cannot accept new records")
	}
	chain := rp.Chains[w.author]
	if len(chain) == 0 {
		if e, ok := rp.Errors[PathOf(w.author)]; ok {
			return fmt.Errorf("identity not valid in this log: %s: %s", w.author, e)
		}
		return errors.New("input missing: genesis " + w.author)
	}
	if head := chain[len(chain)-1].ValueKey(); w.signer == nil || head != w.signer.Public() {
		pub := ""
		if w.signer != nil {
			pub = w.signer.Public()
		}
		return fmt.Errorf("retired key: this copy signs with %s, but the current key of %s is %s; copy local/signing-key from the machine that rotated, or run bean rotate here with the 24 words", pub, w.author, head)
	}
	return nil
}

// canWriteRecord refuses one signed record that the verifier would reject
// for reasons that CheckFile cannot see.
func (l *Loaded) canWriteRecord(r *Record) error {
	if r.IsIdentity() {
		return errors.New("bad type: identity records are written only by init, rotate and restore")
	}
	for _, t := range r.Targets("supersedes") {
		if target, ok := l.Report.Records[t]; ok {
			if IsCoreType(target.Type) {
				return ErrBadTarget
			}
			if target.Author != r.Author {
				return ErrNotAuthor
			}
		}
	}
	if r.Type == "bean.tombstone" {
		if target, ok := l.Report.Records[r.Targets("purges")[0]]; ok {
			if target.IsIdentity() || target.Type == "bean.tombstone" {
				return ErrBadTarget
			}
			if target.Subject != r.Subject {
				return ErrNotSubject
			}
		}
	}
	return nil
}
