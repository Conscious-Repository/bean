package core

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Agents (docs/COMMANDS.md, "AI and agents"). An agent is a secp256k1 key in
// local/agent-key. It writes ordinary claims about the local identity as a
// bare-key author (SPEC.md 6.1: anyone may add a claim; readers decide
// trust), so what an AI logged is never mistaken for what the person
// signed. It cannot supersede or tombstone the person's records: the
// verifier allows that only to the record's author and its subject.
//
// Each machine makes its own agent key, because local/ never syncs. A
// replica can hold an agent key without the person's signing key (an
// agent-only replica, for example a cloud machine that runs an agent).
// The person vouches for an agent with an agent.pair claim. The core does
// not check pairing.

// ErrNoAgent is returned when local/agent-key does not exist.
var ErrNoAgent = errors.New("no agent key: run bean agent init")

// Identity returns the local identity id.
func (s *Store) Identity() (string, error) {
	b, err := os.ReadFile(s.path(fileIdentity))
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotInitialized
	}
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(b))
	if !IsID(id) {
		return "", errors.New("bad field: local/identity")
	}
	return id, nil
}

// identitySecret reads local/identity-secret. An agent-only replica may
// not have it; then it cannot encrypt or read blobs.
func (s *Store) identitySecret() ([]byte, error) {
	b, err := os.ReadFile(s.path(fileSecret))
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("no identity secret on this replica")
	}
	if err != nil {
		return nil, err
	}
	secret, err := hex.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(secret) != 32 {
		return nil, errors.New("bad field: local/identity-secret")
	}
	return secret, nil
}

// Agent is the local agent key and the identity it writes about.
type Agent struct {
	Identity string
	Signer   Signer
	Secret   []byte // nil when this replica has no identity secret
}

// ReadAgent loads local/agent-key.
func (s *Store) ReadAgent() (*Agent, error) {
	id, err := s.Identity()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(s.path(fileAgentKey))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoAgent
	}
	if err != nil {
		return nil, err
	}
	hx, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "secp256k1:")
	raw, err := hex.DecodeString(hx)
	if !ok || err != nil {
		return nil, errors.New("bad field: local/agent-key")
	}
	signer, err := NewSecp256k1Signer(raw)
	if err != nil {
		return nil, errors.New("bad field: local/agent-key")
	}
	a := &Agent{Identity: id, Signer: signer}
	if secret, err := s.identitySecret(); err == nil {
		a.Secret = secret
	}
	return a, nil
}

// AgentInit makes the agent key if it does not exist and returns its
// tagged public key. On a folder without local/identity (a synced copy on
// another machine) it takes identity, or the only genesis in the folder
// when identity is "". When this replica holds the person's signing key,
// it also writes the agent.pair claim and returns its id.
func (s *Store) AgentInit(identity, label string) (string, string, error) {
	if label != "" {
		if err := CheckLabel(label); err != nil {
			return "", "", err // before a key is made
		}
	}
	unlock, err := s.lock()
	if err != nil {
		return "", "", err
	}
	defer unlock()
	have, err := s.Identity()
	switch {
	case errors.Is(err, ErrNotInitialized):
		if identity == "" {
			if identity, err = s.onlyGenesis(); err != nil {
				return "", "", err
			}
		}
		if err := writeFileAtomic(s.path(fileIdentity), []byte(identity+"\n"), 0o600); err != nil {
			return "", "", err
		}
	case err != nil:
		return "", "", err
	case identity != "" && identity != have:
		return "", "", errors.New("local/identity is " + have)
	}
	a, err := s.ReadAgent()
	if errors.Is(err, ErrNoAgent) {
		key, line, kerr := NewKey{Alg: "secp256k1"}.signer()
		if kerr != nil {
			return "", "", kerr
		}
		if err := writeFileAtomic(s.path(fileAgentKey), []byte(line), 0o600); err != nil {
			return "", "", err
		}
		a, err = &Agent{Signer: key}, nil
	}
	if err != nil {
		return "", "", err
	}
	pub := a.Signer.Public()
	// One machine has one agent key, so every assistant that runs here
	// signs as the same agent. A label names the key, not one assistant:
	// a second name would show the first one's records under it. Refuse
	// instead (DECISIONS 209).
	l, err := s.load(false)
	if err != nil {
		return "", "", err
	}
	ps := l.agentPairings(pub)
	live := livePairings(ps)
	if len(live) == 0 && len(ps) > 0 {
		return "", "", fmt.Errorf("you unpaired the agent key of this machine; bean does not trust it again by itself. To trust it again, run on your own machine: bean agent pair %s", Npub(pub))
	}
	if other := otherLabels(live, label); other != "" {
		return "", "", fmt.Errorf("the agent key of this machine is paired as %s. Every assistant on this machine signs with that one key, so bean will not show its records as %q. Use -label %s, or no -label", other, label, strconv.Quote(shownLabel(live[0].label)))
	}
	if len(live) > 0 {
		return pub, live[0].id, nil
	}
	if _, err := s.ReadLocal(); err != nil {
		return pub, "", nil // agent-only: the person pairs it on their own machine
	}
	pairID, err := s.pairWith(l, pub, label, "", "")
	return pub, pairID, err
}

func (s *Store) onlyGenesis() (string, error) {
	l, err := s.load(false)
	if err != nil {
		return "", err
	}
	ids := sortedKeys(l.Report.Chains)
	if len(ids) != 1 {
		return "", errors.New("input missing: one genesis in this folder; sync the log first or pass -identity")
	}
	return ids[0], nil
}

// Pair writes an agent.pair claim signed by the person: they trust this
// key to write claims about them. A current pair claim for the same key is
// returned instead of writing another. Pair refuses a label that differs
// from the one the key has: every record the key wrote would show under
// the new name. A key the person unpaired gets a new pair claim.
func (s *Store) Pair(key, label string) (string, error) {
	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	if _, err := ParseKey(key); err != nil {
		return "", err
	}
	l, err := s.load(false)
	if err != nil {
		return "", err
	}
	live := livePairings(l.agentPairings(key))
	if other := otherLabels(live, label); other != "" {
		return "", fmt.Errorf("this agent key is already paired as %s; bean does not rename it, because every record it wrote would then show as %q", other, label)
	}
	if len(live) > 0 {
		return live[0].id, nil
	}
	return s.pairWith(l, key, label, "", "")
}

// PairPeer writes an agent.pair claim for the key of another person: a
// peer whose signed notes the person takes into their log (docs/SHARE.md,
// "A signed reply"). The claim says that it is a peer and names the
// identity that the key spoke for when the person accepted it. That is the
// person's word: the log does not hold the peer's identity records. The
// rules for a label are those of Pair, and a label is required: a note
// must show under a name. A name belongs to one identity: PairPeer refuses
// a name that a key of another identity, or an agent, has.
//
// head is the id of the last record of the peer's identity chain, as the
// person saw it. A later reply must show a chain that holds it: an older
// view of the chain is a retired key's, and another view is a fork
// (PeerHeads).
func (s *Store) PairPeer(key, label, identity, head string) (string, error) {
	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	if _, err := ParseKey(key); err != nil {
		return "", err
	}
	if label == "" || !IsID(identity) || !IsID(head) {
		return "", errors.New("a peer is paired under a name, with the identity that its key spoke for")
	}
	l, err := s.load(false)
	if err != nil {
		return "", err
	}
	if l.Unpaired(key) {
		return "", errors.New("you unpaired this key: it is not paired again")
	}
	live := livePairings(l.agentPairings(key))
	if other := otherLabels(live, label); other != "" {
		return "", fmt.Errorf("this key is already paired as %s; bean does not rename it, because every record it wrote would then show as %q", other, label)
	}
	if len(live) > 0 {
		return live[0].id, nil
	}
	if l.NameTaken(label, identity) {
		return "", fmt.Errorf("the name %q is taken: a key of another person, or an agent, has it. Give another name", label)
	}
	return s.pairWith(l, key, label, identity, head)
}

// NameTaken reports whether a pairing in force has the name for a key that
// is not a peer's key of this identity: an agent's, or another person's.
func (l *Loaded) NameTaken(label, identity string) bool {
	for _, ps := range l.pairClaims() {
		for _, p := range livePairings(ps) {
			of, _ := l.Report.Records[p.id].Value["identity"].(string)
			if shownLabel(p.label) == label && (!p.peer || of != identity) {
				return true
			}
		}
	}
	return false
}

// PeerHeads returns the heads of an identity's chain that the person saw
// when they paired a key of it as a peer: every one, also of a pairing
// that they took back. A reply of that identity must show a chain that
// holds each of them.
func (l *Loaded) PeerHeads(identity string) []string {
	var out []string
	if l.ID == "" || !IsID(identity) {
		return nil
	}
	for _, id := range sortedKeys(l.Report.Records) {
		r := l.Report.Records[id]
		if r.Type != "agent.pair" || r.Author != l.ID || r.Subject != l.ID || !l.Current(id) {
			continue
		}
		role, _ := r.Value["role"].(string)
		of, _ := r.Value["identity"].(string)
		if head, _ := r.Value["head"].(string); role == "peer" && of == identity && IsID(head) && !slices.Contains(out, head) {
			out = append(out, head)
		}
	}
	return out
}

// Unpair withdraws the person's word for keys: every key that is paired
// under the name, or the one key given. Each pairing in force gets a
// correction that says unpaired, so every copy and every reader obeys it.
// The records that the key signed stay, and show as an unpaired key's. It
// returns the keys that it unpaired.
func (s *Store) Unpair(nameOrKey string) ([]string, error) {
	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	l, err := s.load(false)
	if err != nil {
		return nil, err
	}
	w, err := s.writerFor(l, false)
	if err != nil {
		return nil, err
	}
	var lines []string
	var keys []string
	claims := l.pairClaims()
	for _, key := range sortedKeys(claims) {
		for _, p := range livePairings(claims[key]) {
			if key != nameOrKey && shownLabel(p.label) != nameOrKey {
				continue
			}
			value := map[string]any{}
			for k, v := range l.Report.Records[p.id].Value {
				value[k] = v
			}
			value["status"] = "unpaired"
			lines = append(lines, string(Canonical(map[string]any{
				"type":   "agent.pair",
				"value":  value,
				"times":  map[string]any{"observed": nil},
				"source": map[string]any{"system": "self", "adapter": Adapter, "ref": nil},
				"refs":   []any{map[string]any{"rel": "supersedes", "id": p.id}},
			})))
			if !slices.Contains(keys, key) {
				keys = append(keys, key)
			}
		}
	}
	if len(lines) == 0 {
		return nil, nil
	}
	if _, err := s.appendWith(l, w, strings.NewReader(strings.Join(lines, "\n")+"\n")); err != nil {
		return nil, err
	}
	return keys, nil
}

// Unpaired reports a key that the person paired once and unpaired since,
// with no pairing in force.
func (l *Loaded) Unpaired(key string) bool {
	ps := l.agentPairings(key)
	return len(ps) > 0 && len(livePairings(ps)) == 0
}

// PeerLabel returns the name under which the person paired a key of this
// identity as a peer, or "" when they paired none. A peer who changed
// their key keeps their name.
func (l *Loaded) PeerLabel(identity string) string {
	if l.ID == "" || !IsID(identity) {
		return ""
	}
	for _, id := range sortedKeys(l.Report.Records) {
		r := l.Report.Records[id]
		if r.Type != "agent.pair" || r.Author != l.ID || r.Subject != l.ID || !l.Current(id) {
			continue
		}
		role, _ := r.Value["role"].(string)
		of, _ := r.Value["identity"].(string)
		st, _ := r.Value["status"].(string)
		if label, _ := r.Value["label"].(string); role == "peer" && of == identity && st != "unpaired" && label != "" {
			return label
		}
	}
	return ""
}

func (s *Store) pairWith(l *Loaded, key, label, peerOf, head string) (string, error) {
	if label != "" {
		if err := CheckLabel(label); err != nil {
			return "", err
		}
	}
	// An agent does not take the name of a peer: its records would show
	// beside that person's under one name. PairPeer checks the other way.
	if peerOf == "" {
		for _, ps := range l.pairClaims() {
			for _, p := range livePairings(ps) {
				if p.peer && label != "" && p.label == label {
					return "", fmt.Errorf("the name %q is the name of a peer: give the agent another one", label)
				}
			}
		}
	}
	w, err := s.writerFor(l, false)
	if err != nil {
		return "", err
	}
	value := map[string]any{"key": key}
	if label != "" {
		value["label"] = label
	}
	if peerOf != "" {
		value["role"], value["identity"], value["head"] = "peer", peerOf, head
	}
	obj := map[string]any{
		"type":   "agent.pair",
		"value":  value,
		"times":  map[string]any{"observed": nil},
		"source": map[string]any{"system": "self", "adapter": Adapter, "ref": nil},
	}
	res, err := s.appendWith(l, w, strings.NewReader(string(Canonical(obj))+"\n"))
	if err != nil {
		return "", err
	}
	if len(res.IDs) != 1 {
		return "", errors.New("not added")
	}
	return res.IDs[0], nil
}

// pairing is one current agent.pair claim of the person.
type pairing struct {
	id, label string // label is "" when the claim has none
	unpaired  bool
	peer      bool // the key of another person, not of an agent
}

// pairClaims maps each key to the local person's current agent.pair
// claims for it, in id order. Only the local person's pairings count:
// another identity in the folder cannot name an agent for this person.
func (l *Loaded) pairClaims() map[string][]pairing {
	out := map[string][]pairing{}
	if l.ID == "" {
		return out
	}
	for id, r := range l.Report.Records {
		if r.Type != "agent.pair" || r.Author != l.ID || r.Subject != l.ID || !l.Current(id) || r.ValueKey() == "" {
			continue
		}
		label, _ := r.Value["label"].(string)
		st, _ := r.Value["status"].(string)
		role, _ := r.Value["role"].(string)
		out[r.ValueKey()] = append(out[r.ValueKey()], pairing{id: id, label: label, unpaired: st == "unpaired", peer: role == "peer"})
	}
	for _, ps := range out {
		slices.SortFunc(ps, func(a, b pairing) int { return strings.Compare(a.id, b.id) })
	}
	return out
}

func (l *Loaded) agentPairings(key string) []pairing { return l.pairClaims()[key] }

func livePairings(ps []pairing) []pairing {
	var out []pairing
	for _, p := range ps {
		if !p.unpaired {
			out = append(out, p)
		}
	}
	return out
}

// otherLabels names the labels of live, quoted, when label is not one of
// them. It returns "" when label is "" or a claim has it: a key paired under
// two names, from two of the person's machines, answers to each.
func otherLabels(live []pairing, label string) string {
	if label == "" || len(live) == 0 {
		return ""
	}
	var names []string
	for _, p := range live {
		if p.label == label {
			return ""
		}
		names = append(names, strconv.Quote(shownLabel(p.label)))
	}
	return strings.Join(names, " and ")
}

// MaxLabel is the most bytes that the name of a paired key may have.
const MaxLabel = 64

// CheckLabel reports why label cannot be the name of a paired key, or nil.
// A name shows in every listing, between brackets and beside other names,
// and it tells one signer from another. So it holds only what cannot fake
// a second name or look like another: the letters a to z in both cases,
// digits, single spaces, and . ' _ - @. It does not start with the word
// peer, which bean writes before the name of another person.
func CheckLabel(label string) error {
	bad := func(why string) error {
		return fmt.Errorf("%s is not a name that bean can show: %s", strconv.QuoteToASCII(label), why)
	}
	switch {
	case label == "" || len(label) > MaxLabel:
		return bad(fmt.Sprintf("a name has 1 to %d characters", MaxLabel))
	case strings.TrimSpace(label) != label || strings.Contains(label, "  "):
		return bad("a name has no space at its start or end, and no two spaces together")
	}
	for i := 0; i < len(label); i++ {
		c := label[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte(" .'_-@", c) >= 0) {
			return bad("a name holds the letters a to z, digits, spaces and . ' _ - @ only")
		}
	}
	if lower := strings.ToLower(label); lower == "peer" || strings.HasPrefix(lower, "peer ") {
		return bad("a name does not start with the word peer, which bean writes before the name of another person")
	}
	return nil
}

// ShownLabel is how readers name a paired key: its label, or "agent" when
// it has none. A label that breaks the rule of CheckLabel can still be in
// a record, of an older bean or written by hand. It shows in quotation
// marks with every other character escaped, so it cannot pass for a name.
func ShownLabel(label string) string {
	switch {
	case label == "":
		return "agent"
	case CheckLabel(label) != nil:
		return strconv.QuoteToASCII(label)
	}
	return label
}

func shownLabel(label string) string { return ShownLabel(label) }

// PairedLabels maps each key that the subject paired with a current
// agent.pair claim to its label ("agent" when it has none). A key the
// person paired twice under different names, for example on two of their
// machines, shows both names, in id order, never one picked at random.
// The name of a peer, another person, has the word peer before it.
func (l *Loaded) PairedLabels() map[string]string { return l.pairLabels(true) }

// FileLabels is PairedLabels without the peers: the keys whose records may
// name a file of the person. A peer is another person, and a peer's record
// names no file, so it unlocks none (DECISIONS 293). A key that is paired
// both as an agent and as a peer counts as a peer.
func (l *Loaded) FileLabels() map[string]string { return l.pairLabels(false) }

// PairedAsAgent returns the name under which a pairing in force makes key
// an agent of the person, or "" when there is none. A key that is also
// paired as a peer still counts: two of the person's machines can pair one
// key in the two ways.
func (l *Loaded) PairedAsAgent(key string) string {
	for _, p := range livePairings(l.agentPairings(key)) {
		if !p.peer {
			return shownLabel(p.label)
		}
	}
	return ""
}

func (l *Loaded) pairLabels(peers bool) map[string]string {
	out := map[string]string{}
	for key, ps := range l.pairClaims() {
		var names []string
		live := livePairings(ps)
		if !peers && slices.ContainsFunc(live, func(p pairing) bool { return p.peer }) {
			continue
		}
		for _, p := range live {
			name := shownLabel(p.label)
			// A peer is another person. Their records show as a peer's
			// wherever a name shows, so nobody takes them for an agent's
			// or for the person's own.
			if p.peer {
				name = "peer " + name
			}
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			out[key] = strings.Join(names, ", ")
		}
	}
	return out
}

// AppendAsAgent is Append signed by the local agent key, with the agent's
// key as the author and the local identity as the default subject.
func (s *Store) AppendAsAgent(in io.Reader) (AppendResult, error) { return s.appendAs(in, true, false) }
