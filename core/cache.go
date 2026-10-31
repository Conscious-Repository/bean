package core

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The object cache, local/cache/objects, works like git's index. Records
// are named by their hash and never edited, so a file whose size and
// modification time match the cache still holds the bytes this replica
// already proved, and ordinary commands skip its signature check. Every
// other check still runs. bean verify ignores the cache, proves every
// file, and rebuilds it: verify is the fsck.
//
// Each line is "<path on disk>\t<size>\t<mtime ns>\t<file bytes>". Only
// canonical records are cached, and canonical JSON has no raw tab or
// newline. The cache is never the truth (SPEC.md 7.10): a missing or
// garbled cache only costs time.

const fileObjCache = "local/cache/objects"

// racyWindow keeps files modified this recently out of the cache, as git
// does with "racily clean" entries: an edit in the same clock tick as the
// cache write would otherwise keep a matching stamp.
const racyWindow = 2 * time.Second

type stamp struct{ size, mtime int64 }

// diskObject is one record file on disk.
type diskObject struct {
	rel   string // path on disk, relative to the log
	path  string // the objects/ path the verifier sees
	stamp stamp
}

// objectSet is what readObjects found: the files on disk, the hidden
// ids, the record files as the verifier sees them, the paths this replica
// already proved, entries that are not record files, and the cache size.
type objectSet struct {
	disk     []diskObject
	hidden   map[string]bool
	files    map[string][]byte
	proven   map[string]bool
	bad      []string // entries of objects/ that are not plain files (listShards)
	cacheLen int
}

// listObjects lists objects/ and local/hidden/ with their stamps, the ids
// in local/hidden/, and the entries of objects/ that are not record
// files. objects/ wins when an id is in both; the id still counts as
// hidden.
func (s *Store) listObjects() ([]diskObject, map[string]bool, []string, error) {
	var out []diskObject
	seen := map[string]bool{}
	hiddenIDs := map[string]bool{}
	add := func(rel, path string, e os.DirEntry) error {
		if seen[path] {
			return nil
		}
		info, err := e.Info()
		if errors.Is(err, os.ErrNotExist) {
			return errMoved
		}
		if err != nil {
			return err
		}
		seen[path] = true
		out = append(out, diskObject{rel, path, stamp{info.Size(), info.ModTime().UnixNano()}})
		return nil
	}
	files, bad, err := listShards(s.Dir, dirObjects)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, f := range files {
		if err := add(f.rel, f.rel, f.e); err != nil {
			return nil, nil, nil, err
		}
	}
	hidden, err := os.ReadDir(s.path(dirHidden))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil, err
	}
	for _, e := range hidden {
		name := e.Name()
		if !e.Type().IsRegular() || !strings.HasSuffix(name, ".json") || len(name) != 69 {
			continue
		}
		hiddenIDs["sha256:"+name[:64]] = true
		if err := add(dirHidden+"/"+name, PathOf("sha256:"+name[:64]), e); err != nil {
			return nil, nil, nil, err
		}
	}
	return out, hiddenIDs, bad, nil
}

// errMoved: a listed file was gone when read. Another bean process hid or
// restored it in between; list again.
var errMoved = errors.New("record moved while reading")

type cached struct {
	stamp stamp
	data  []byte
}

func (s *Store) readObjCache() map[string]cached {
	out := map[string]cached{}
	f, err := os.Open(s.path(fileObjCache))
	if err != nil {
		return out
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return out // a torn last line is dropped
		}
		parts := bytes.SplitN(line[:len(line)-1], []byte{'\t'}, 4)
		if len(parts) != 4 {
			continue
		}
		size, err1 := strconv.ParseInt(string(parts[1]), 10, 64)
		mtime, err2 := strconv.ParseInt(string(parts[2]), 10, 64)
		if err1 != nil || err2 != nil || int64(len(parts[3])) != size {
			continue
		}
		out[string(parts[0])] = cached{stamp{size, mtime}, parts[3]}
	}
}

// readObjects reads objects/ and local/hidden/. full ignores the cache.
func (s *Store) readObjects(full bool) (*objectSet, error) {
	disk, hidden, bad, err := s.listObjects()
	if err != nil {
		return nil, err
	}
	cache := map[string]cached{}
	if !full {
		cache = s.readObjCache()
	}
	files := make(map[string][]byte, len(disk))
	proven := map[string]bool{}
	var todo []diskObject
	for _, d := range disk {
		if c, ok := cache[d.rel]; ok && c.stamp == d.stamp {
			files[d.path] = c.data
			proven[d.path] = true
		} else {
			todo = append(todo, d)
		}
	}
	// Read the rest on all CPUs; a log can have tens of thousands of files.
	data := make([][]byte, len(todo))
	errs := make([]error, len(todo))
	parallel(len(todo), func(i int) { data[i], errs[i] = os.ReadFile(s.path(todo[i].rel)) })
	for i, d := range todo {
		if errors.Is(errs[i], os.ErrNotExist) {
			return nil, errMoved
		}
		if errs[i] != nil {
			return nil, errs[i]
		}
		files[d.path] = data[i]
	}
	return &objectSet{disk, hidden, files, proven, bad, len(cache)}, nil
}

// writeObjCache stores every record that passed steps 1 to 15, unless
// that is exactly what the cache already held (every entry was used and
// none is new). It is best effort: a log on read-only media still loads.
func (s *Store) writeObjCache(o *objectSet, rp *Report) {
	racy := time.Now().Add(-racyWindow).UnixNano() // file times are wall-clock, not $BEAN_NOW
	good := map[string]bool{}
	for _, r := range rp.Records {
		good[r.Path] = true
	}
	sort.Slice(o.disk, func(i, j int) bool { return o.disk[i].rel < o.disk[j].rel })
	var buf bytes.Buffer
	n := 0
	for _, d := range o.disk {
		if !good[d.path] || d.stamp.mtime > racy {
			continue
		}
		n++
		buf.WriteString(d.rel + "\t" + strconv.FormatInt(d.stamp.size, 10) + "\t" + strconv.FormatInt(d.stamp.mtime, 10) + "\t")
		buf.Write(o.files[d.path])
		buf.WriteByte('\n')
	}
	if n == len(o.proven) && o.cacheLen == len(o.proven) {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path(fileObjCache)), 0o700); err == nil {
		_ = writeFileAtomic(s.path(fileObjCache), buf.Bytes(), 0o600)
	}
}
