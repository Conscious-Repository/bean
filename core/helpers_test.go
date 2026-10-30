package core

import "testing"

// writeRaw signs one partial record with s's local signing key and writes
// it into objects/ without any of the write checks (DECISIONS 108). It
// stands for a record written by an older build, another tool, or a thief.
func writeRaw(t *testing.T, s *Store, partialLine string) string {
	t.Helper()
	loc, err := s.ReadLocal()
	if err != nil {
		t.Fatal(err)
	}
	v, err := Parse([]byte(partialLine))
	if err != nil {
		t.Fatal(err)
	}
	obj := v.(map[string]any)
	obj["v"] = 0.0
	obj["nonce"] = NewNonce()
	obj["author"] = loc.Identity
	obj["kid"] = loc.Signer.Public()
	if _, has := obj["subject"]; !has {
		obj["subject"] = loc.Identity
	}
	times, _ := obj["times"].(map[string]any)
	if times == nil {
		times = map[string]any{"observed": nil}
		obj["times"] = times
	}
	times["imported"] = s.nowString()
	src := obj["source"].(map[string]any)
	if _, has := src["ref"]; !has {
		src["ref"] = nil
	}
	id, b, err := SignRecord(obj, loc.Signer)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(s.path(PathOf(id)), b, 0o600); err != nil {
		t.Fatal(err)
	}
	return id
}
