package core

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Files and directories (SPEC.md 7.1).
const (
	dirObjects    = "objects"
	dirBlobs      = "blobs"
	dirLocal      = "local"
	dirHidden     = "local/hidden"
	dirCache      = "local/cache"
	fileIdentity  = "local/identity"
	fileSecret    = "local/identity-secret"
	fileSignKey   = "local/signing-key"
	fileHolds     = "local/holds.json"
	filePurgedRef = "local/purged-refs.jsonl"
	fileIndex     = "local/cache/index.json"
)

// writeFileAtomic writes data to a temporary name in the same directory and
// renames it. It creates parent directories with mode 0700.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(path), ".tmp-"+hex.EncodeToString(rnd[:]))
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// writeIfAbsent writes the file unless it exists (7.2 step 6, 7.3 step 3).
// It reports whether it wrote.
func writeIfAbsent(path string, data []byte, mode os.FileMode) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return true, writeFileAtomic(path, data, mode)
}

// listShardFiles lists files under <root>/<dir>/<2 hex>/<name> and returns
// log-relative paths, sorted.
func listShardFiles(root, dir string) ([]string, error) {
	var out []string
	shards, err := os.ReadDir(filepath.Join(root, dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, s := range shards {
		if !s.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, dir, s.Name()))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if f.IsDir() || strings.HasPrefix(f.Name(), ".tmp-") {
				continue
			}
			out = append(out, dir+"/"+s.Name()+"/"+f.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// readRecordFiles loads every objects/ file as path -> bytes.
func readRecordFiles(root string) (map[string][]byte, error) {
	paths, err := listShardFiles(root, dirObjects)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(paths))
	for _, p := range paths {
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			return nil, err
		}
		out[p] = b
	}
	return out, nil
}

// readHiddenFiles loads local/hidden/<64hex>.json as the objects/ path each
// file would have, so the verifier sees hidden targets as stored records.
func readHiddenFiles(root string) (map[string][]byte, error) {
	entries, err := os.ReadDir(filepath.Join(root, dirHidden))
	if errors.Is(err, os.ErrNotExist) {
		return map[string][]byte{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || len(name) != 69 {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, dirHidden, name))
		if err != nil {
			return nil, err
		}
		out[PathOf("sha256:"+name[:64])] = b
	}
	return out, nil
}

func hiddenPath(root, id string) string {
	return filepath.Join(root, dirHidden, id[7:]+".json")
}

// blobIDOfPath converts blobs/ab/cd... to an id, or "" when the path is not
// well-formed.
func blobIDOfPath(p string) string {
	parts := strings.Split(p, "/")
	if len(parts) != 3 || len(parts[1]) != 2 || len(parts[2]) != 62 {
		return ""
	}
	id := "sha256:" + parts[1] + parts[2]
	if !IsID(id) {
		return ""
	}
	return id
}
