package core

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
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
	fileAgentKey  = "local/agent-key"
	fileHolds     = "local/holds.json"
	filePurgedRef = "local/purged-refs.jsonl"
	fileIndex     = "local/cache/index.json"
	fileLock      = "local/lock"
)

// writeFileAtomic writes data to a temporary name in the same directory,
// flushes it to disk, renames it, and flushes the directory, so a crash
// leaves either the old state or the whole new file, never an empty one.
// It creates parent directories with mode 0700.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".tmp-"+hex.EncodeToString(rnd[:]))
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = syncFile(f)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return syncDir(dir)
}

// syncDir flushes a directory entry after a rename. Some systems cannot
// open a directory for sync; that is not an error there.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer d.Close()
	if err := syncFile(d); err != nil && !errors.Is(err, os.ErrInvalid) && !errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.ENOTSUP) {
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
			if !f.Type().IsRegular() || strings.HasPrefix(f.Name(), ".tmp-") {
				continue // a pipe here would hang a read, and rsync can bring one
			}
			out = append(out, dir+"/"+s.Name()+"/"+f.Name())
		}
	}
	sort.Strings(out)
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
