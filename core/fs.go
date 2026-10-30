package core

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
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
)

// writeFileAtomic writes data to a temporary name in the same directory,
// flushes it to disk, renames it, and flushes the directory, so a crash
// leaves either the old state or the whole new file, never an empty one.
// It creates parent directories with mode 0700.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := mkdirAllSync(dir); err != nil {
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

// mkdirAllSync is os.MkdirAll with mode 0700 that also flushes the parent
// of every directory it creates, so a new shard directory survives a crash
// together with the file inside it.
func mkdirAllSync(dir string) error {
	var made []string
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		made = append(made, d)
		if filepath.Dir(d) == d {
			break
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for _, d := range made {
		if err := syncDir(filepath.Dir(d)); err != nil {
			return err
		}
	}
	return nil
}

// appendLineSync appends one line to a file and flushes it.
func appendLineSync(path, line string) error {
	if err := mkdirAllSync(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(line + "\n")
	if err == nil {
		err = syncFile(f)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// removeStaleTemps deletes temporary files that a crashed writer left in
// dir and its shard directories. Only files older than an hour go, so a
// write in progress in another process is never touched.
func removeStaleTemps(dirs ...string) {
	cutoff := time.Now().Add(-time.Hour)
	var walk func(d string, depth int)
	walk = func(d string, depth int) {
		entries, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, e := range entries {
			p := filepath.Join(d, e.Name())
			if e.IsDir() {
				if depth == 0 {
					walk(p, 1)
				}
				continue
			}
			if !strings.HasPrefix(e.Name(), ".tmp-") {
				continue
			}
			if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
				os.Remove(p)
			}
		}
	}
	for _, d := range dirs {
		walk(d, 0)
	}
}

// syncDir flushes a directory entry after a rename or a create. Some
// systems cannot sync a directory; that is not an error there.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
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

// shardFile is one plain file at <dir>/<shard>/<name>.
type shardFile struct {
	rel string // log-relative path
	e   os.DirEntry
}

// listShards lists <root>/<dir>/<shard>/<name>, sorted. Names that start
// with "." are skipped at both levels: temporary files of a writer
// (.tmp-) and what operating systems and sync tools leave (.DS_Store,
// iCloud .icloud stubs, .stversions). Every other entry that is not a
// plain file two levels down (a symlink, a directory, a device, a file
// one level down) is returned in bad, unread: it is reported, never
// skipped in silence and never opened (a FIFO would block).
func listShards(root, dir string) (files []shardFile, bad []string, err error) {
	shards, err := os.ReadDir(filepath.Join(root, dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	for _, sh := range shards {
		if strings.HasPrefix(sh.Name(), ".") {
			continue
		}
		if !sh.IsDir() {
			bad = append(bad, dir+"/"+sh.Name())
			continue
		}
		entries, err := os.ReadDir(filepath.Join(root, dir, sh.Name()))
		if err != nil {
			return nil, nil, err
		}
		for _, e := range entries {
			rel := dir + "/" + sh.Name() + "/" + e.Name()
			switch {
			case strings.HasPrefix(e.Name(), "."):
			case e.Type().IsRegular():
				files = append(files, shardFile{rel, e})
			default:
				bad = append(bad, rel)
			}
		}
	}
	return files, bad, nil
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
