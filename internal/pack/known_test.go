package pack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
)

// held is the SHA-256 of what bean holds of each pack on record, and of
// the texts that a claim may have. A check of a saved comparison reads
// both. So a change to either changes how a record reads that a person
// saved long ago. A pack on record is never changed and never taken off
// (DECISIONS 310, 319): a new pack is a new row here. A change to a row
// that is here is a change to old records, and needs an entry in
// UPGRADING.md.
var held = map[string]string{
	"10555d6769421d4305cbcd4ff14960d8b966ddd829a50581a9584a60a84d29e2": "85a9c365f62f4ad15de88b79cd4f27b9bafe5c0966e3d2bf800813e72ebda38f",
	"5cae07db64295d565ffae7c251c17fdb9f043dc0638868557275f8b0b0311efa": "65754cc3eb13f0e710de3ad86bb787459ace755525449a8660c73e9dfa53f501",
	// This row left out the least n of a cell on 2026-11-19 (DECISIONS 333,
	// UPGRADING.md 0.8.2): a cell has none now, and no text of a claim changed.
	"the texts of a claim": "f44279528f561dcf2b97cf59a82e7b134b8cb58888d753fcf00384b31b6212dc",
}

func digestOf(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestWhatIsOnRecordNeverChanges(t *testing.T) {
	got := map[string]string{
		"the texts of a claim": digestOf([]any{CaveatCohort, fmt.Sprintf(CaveatSmall, SmallN), CaveatNotOnRecord, CaveatAuthor, MethodID, MethodVersion, SmallN, Format, Kind}),
	}
	for digest, k := range Known {
		got[digest] = digestOf(k)
	}
	for name, want := range held {
		if got[name] != want {
			t.Errorf("%s: what bean holds has the SHA-256 %s, and the one that is held here is %s", name, got[name], want)
		}
	}
	for name := range got {
		if _, ok := held[name]; !ok {
			t.Errorf("%s is on record, and has no row here: add %q", name, got[name])
		}
	}
}
