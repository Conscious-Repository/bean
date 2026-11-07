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

// Agents (docs/ROADMAP.md, chat logging). An agent is a secp256k1 key in
// local/agent-key. It writes ordinary claims about the local identity as a
// bare-key author (SPEC.md 6.1: anyone may add a claim; readers decide
// trust), so what an AI logged is never mistaken for what the person
// signed. It cannot supersede or tombstone the person's records: the
// verifier allows that only to the record's author and its subject.
//
// Each machine makes its own agent key, because local/ never syncs. A
// replica can hold an agent key without the person's signing key (an
// agent-only replica, for example a cloud machine that runs an agent).
// The person vouches for an agent with an agent.pair claim, the same shape
// as device.pair (SPEC.md 11.3). The core does not check pairing.

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
	pairID, err := s.pairWith(l, pub, label)
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
	return s.pairWith(l, key, label)
}

func (s *Store) pairWith(l *Loaded, key, label string) (string, error) {
	w, err := s.writerFor(l, false)
	if err != nil {
		return "", err
	}
	value := map[string]any{"key": key}
	if label != "" {
		value["label"] = label
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
		out[r.ValueKey()] = append(out[r.ValueKey()], pairing{id: id, label: label, unpaired: st == "unpaired"})
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

// otherLabels names the labels of live, quoted, when one of them differs
// from label. It returns "" when label is "" or every claim has it.
func otherLabels(live []pairing, label string) string {
	if label == "" {
		return ""
	}
	var names []string
	differs := false
	for _, p := range live {
		differs = differs || p.label != label
		names = append(names, strconv.Quote(shownLabel(p.label)))
	}
	if !differs {
		return ""
	}
	return strings.Join(names, " and ")
}

// shownLabel is how readers name a paired key.
func shownLabel(label string) string {
	if label == "" {
		return "agent"
	}
	return label
}

// AgentLabels maps each key that the subject paired with a current
// agent.pair claim to its label ("agent" when it has none). A key the
// person paired twice under different names, for example on two of their
// machines, shows both names, in id order, never one picked at random.
func (l *Loaded) AgentLabels() map[string]string {
	out := map[string]string{}
	for key, ps := range l.pairClaims() {
		var names []string
		for _, p := range livePairings(ps) {
			if name := shownLabel(p.label); !slices.Contains(names, name) {
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
