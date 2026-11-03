package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"

	"github.com/Conscious-Repository/bean/core"
)

// archiveRecord preserves each source independently of specimen/test dedupe.
// Its digest describes plaintext; blob identifies the caller-preserved encrypted
// source. This does not verify the caller's blob reference or the lab's origin.
func archiveRecord(raw []byte, blob string) map[string]any {
	if blob == "" {
		return nil
	}
	digest := sha256.Sum256(raw)
	media := "text/plain"
	if bytes.HasPrefix(raw, []byte("%PDF")) {
		media = "application/pdf"
	}
	value := map[string]any{"blob": blob}
	return map[string]any{
		"type":  "lab.report",
		"value": value,
		"times": map[string]any{"observed": nil},
		"source": map[string]any{
			"system": system, "adapter": version,
			"ref_input": core.RefInputTuple(system, "lab.report", nil, value, 0),
		},
		"context": map[string]any{
			"sha256": hex.EncodeToString(digest[:]), "bytes": len(raw),
			"assurance": "unsigned-report",
		},
		"blobs": []any{map[string]any{"hash": blob, "role": "raw-export", "media_type": media}},
	}
}
