package core

import (
	"bytes"
	"testing"
)

// FuzzTooNewGate: rotations, then records of an unknown v by keys picked
// from the input, then a rotation if the input says so. The gate (8.2, K1)
// and the rotation past a head-key halt (C29) must follow the model.
func FuzzTooNewGate(f *testing.F) {
	f.Add([]byte{0, 0, 1})
	f.Add([]byte{2, 1, 3, 0})
	f.Add([]byte{1, 2, 4, 1})
	f.Add([]byte{3, 0, 2, 5, 1})
	f.Fuzz(func(t *testing.T, in []byte) {
		if len(in) < 2 || len(in) > 8 {
			return
		}
		s, id, words := initWithWords(t)
		seed, err := WordsToSeed(words)
		if err != nil {
			t.Fatal(err)
		}
		appendLines(t, s, partial("hr", 1, nil))
		keys := []Signer{s.mustLocal(t).Signer} // signing keys, oldest first
		rots := int(in[0] % 4)
		for i := 0; i < rots; i++ {
			if _, err := s.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
				t.Fatal(err)
			}
			keys = append(keys, s.mustLocal(t).Signer)
		}
		head, recovery, ahead := false, false, false
		for _, b := range in[1 : len(in)-1] {
			switch k := int(b % 6); {
			case k == 0: // the head key
				futureRecord(t, s, id, keys[len(keys)-1])
				head = true
			case k == 1: // a retired key, or the head when there is none
				futureRecord(t, s, id, keys[int(b/6)%len(keys)])
				if int(b/6)%len(keys) == len(keys)-1 {
					head = true
				}
			case k == 2: // a used or the next recovery key
				futureRecord(t, s, id, RecoverySigner(seed, int(b/6)%(rots+1)))
				recovery = true
			case k == 3: // a recovery key no node commits to yet; the next
				// succession will commit to it, so going past fails closed
				futureRecord(t, s, id, RecoverySigner(seed, rots+1))
				ahead = true
			default: // a stranger
				futureRecord(t, s, id, NewEd25519Signer(bytes.Repeat([]byte{b}, 32)))
			}
		}
		l, err := s.Load()
		if err != nil {
			t.Fatal(err)
		}
		if got := contains(l.Report.TooNew, id); got != (head || recovery) {
			t.Fatalf("too new %v, want %v (head %v recovery %v)", got, head || recovery, head, recovery)
		}
		if got := len(l.Report.HeadHalt[id]) > 0; got != (head && !recovery) {
			t.Fatalf("head halt %v, want %v", got, head && !recovery)
		}
		if in[len(in)-1]%2 == 0 {
			return
		}
		before := fileSet(t, s)
		_, err = s.Rotate(words, Seals{All: true}, NewKey{})
		switch {
		case recovery || (head && ahead):
			if err == nil {
				t.Fatal("rotated, but the identity stays halted")
			}
			if fileSet(t, s) != before {
				t.Fatal("refused rotation wrote files")
			}
		case ahead:
			// Not halted now; after a plain rotation the record gates.
			if err != nil {
				t.Fatalf("rotate: %v", err)
			}
			mustTooNew(t, s, id, true)
		case err != nil:
			t.Fatalf("rotate: %v", err)
		default:
			if l := mustTooNew(t, s, id, false); len(l.Report.Errors) != 0 {
				t.Fatalf("errors after rotation: %v", l.Report.Errors)
			}
		}
	})
}
