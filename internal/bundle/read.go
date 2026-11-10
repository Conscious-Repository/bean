package bundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Conscious-Repository/bean/core"
)

const notRecord = "%s is not a record file: not a plain file, or too large"

// maxRecords is the most that the records of a bundle in memory may hold
// together.
const maxRecords = 1 << 30

// ReadDir reads a bundle from a folder. It reads the record files, plain
// files two levels below objects/ as in a log, and lists files/. It reads
// nothing else of the folder, opens nothing that is not a plain file,
// follows no link out of the folder, and reads no file larger than a record
// can be. Names that start with a dot are not part of a bundle, as in a
// log: a file browser leaves such files behind.
func ReadDir(dir string) Input {
	var in Input
	fatal := func(format string, a ...any) Input {
		return Input{Fatal: []string{fmt.Sprintf(format, a...)}}
	}
	if info, err := os.Lstat(filepath.Join(dir, "objects")); err != nil || !info.IsDir() {
		return fatal("%s is not a bundle: it has no objects folder", dir)
	}
	top, err := os.ReadDir(dir)
	if err != nil {
		return fatal("the folder cannot be read: %s", err)
	}
	// Anything else in the folder is not part of the bundle. It is never
	// read, so it cannot change the verdict, and it is named: pointing bean
	// at a bundle with -dir leaves a local folder there.
	for _, e := range top {
		switch name := e.Name(); {
		case strings.HasPrefix(name, "."):
		case name == "objects":
		case name == "files":
			if !e.IsDir() {
				return fatal("files is not a folder: a link or a file in its place is not followed")
			}
		default:
			in.Ignored = append(in.Ignored, name)
		}
	}
	in.Records = map[string][]byte{}
	shards, err := os.ReadDir(filepath.Join(dir, "objects"))
	if err != nil {
		return fatal("the records cannot be read: %s", err)
	}
	for _, sh := range shards {
		if strings.HasPrefix(sh.Name(), ".") {
			continue
		}
		if !sh.IsDir() {
			in.Problems = append(in.Problems, fmt.Sprintf(notRecord, "objects/"+sh.Name()))
			continue
		}
		entries, err := os.ReadDir(filepath.Join(dir, "objects", sh.Name()))
		if err != nil {
			return fatal("the records cannot be read: %s", err)
		}
		for _, e := range entries {
			rel := "objects/" + sh.Name() + "/" + e.Name()
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			info, err := e.Info()
			if err != nil || !e.Type().IsRegular() || info.Size() > core.MaxSuccessionSize {
				in.Problems = append(in.Problems, fmt.Sprintf(notRecord, rel))
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, "objects", sh.Name(), e.Name()))
			if err != nil {
				return fatal("the records cannot be read: %s", err)
			}
			in.Records[rel] = data
		}
	}
	found, err := os.ReadDir(filepath.Join(dir, "files"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		in.Problems = append(in.Problems, fmt.Sprintf("the files cannot be read: %s", err))
	}
	for _, f := range found {
		if strings.HasPrefix(f.Name(), ".") {
			continue
		}
		file := File{Name: f.Name(), Regular: f.Type().IsRegular()}
		if info, err := f.Info(); err == nil {
			file.Size = info.Size()
		} else {
			file.Size = -1
		}
		path := filepath.Join(dir, "files", f.Name())
		file.Digest = func() string {
			r, err := os.Open(path)
			if err != nil {
				return ""
			}
			defer r.Close()
			return digestOf(r)
		}
		in.Files = append(in.Files, file)
	}
	return in
}

// digestOf is the SHA-256 of what r holds, in hex, or "" when it cannot be
// read.
func digestOf(r io.Reader) string {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// source is one file of a bundle that is not on a disk: its size as
// stated, whether it is a plain file, and how to read it.
type source struct {
	size    int64
	regular bool
	open    func() (io.ReadCloser, error)
}

// FromFiles reads a bundle from files already in memory, each under its
// path with "/" between the names, as a browser hands over a chosen
// folder. The bundle may be the whole of it or one folder inside it.
func FromFiles(name string, files map[string][]byte) Input {
	src := map[string]source{}
	for path, data := range files {
		src[path] = source{size: int64(len(data)), regular: true, open: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(data)), nil
		}}
	}
	return from(name, src)
}

// FromZip reads a bundle from a zip file in memory. Nothing is unpacked to
// a disk, and no entry is read past the size it states.
func FromZip(name string, data []byte) Input {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Input{Fatal: []string{fmt.Sprintf("%s cannot be read as a zip file", name)}}
	}
	src := map[string]source{}
	for _, f := range zr.File {
		path := strings.ReplaceAll(f.Name, `\`, "/")
		if strings.HasSuffix(path, "/") {
			continue // a folder
		}
		if _, twice := src[path]; twice {
			return Input{Fatal: []string{fmt.Sprintf("%s holds two files with one name: which one a reader sees depends on the tool", name)}}
		}
		size := int64(f.UncompressedSize64)
		src[path] = source{size: size, regular: f.Mode().IsRegular(), open: func() (io.ReadCloser, error) {
			r, err := f.Open()
			if err != nil {
				return nil, err
			}
			// One byte more than stated, so that a longer entry reads as
			// another size and not as the stated one.
			return struct {
				io.Reader
				io.Closer
			}{io.LimitReader(r, size+1), r}, nil
		}}
	}
	return from(name, src)
}

// from finds the bundle among files that are not on a disk, and reads it
// as ReadDir reads a folder.
func from(name string, src map[string]source) Input {
	// The bundle's folder is the one that holds objects/<two names>. A zip
	// made on a Mac carries a second tree of its own, which is not one.
	roots := map[string]bool{}
	for path := range src {
		if strings.HasPrefix(path, "__MACOSX/") {
			continue
		}
		if i := strings.LastIndex("/"+path, "/objects/"); i >= 0 {
			if rest := path[i+len("objects/"):]; strings.Count(rest, "/") == 1 {
				roots[path[:i]] = true
			}
		}
	}
	// A folder inside the bundle that looks like one is not another bundle.
	sorted := slices.Sorted(maps.Keys(roots))
	if len(sorted) == 0 {
		return Input{Fatal: []string{fmt.Sprintf("%s is not a bundle: it has no objects folder", name)}}
	}
	root := sorted[0]
	for _, other := range sorted[1:] {
		if !strings.HasPrefix(other, root) {
			return Input{Fatal: []string{fmt.Sprintf("%s holds more than one bundle: choose the folder of one", name)}}
		}
	}
	in := Input{Records: map[string][]byte{}}
	ignored, notPlain, files := map[string]bool{}, map[string]bool{}, map[string]File{}
	var total int64
	for _, path := range slices.Sorted(maps.Keys(src)) {
		if !strings.HasPrefix(path, root) {
			continue
		}
		s := src[path]
		parts := strings.Split(path[len(root):], "/")
		// A name that starts with a dot is skipped where ReadDir skips one.
		inside := parts[0] == "objects" || parts[0] == "files"
		if dot := func(i int) bool { return len(parts) > i && strings.HasPrefix(parts[i], ".") }; dot(0) || inside && (dot(1) || parts[0] == "objects" && dot(2)) {
			continue
		}
		switch {
		case parts[0] == "objects" && len(parts) == 3 && s.regular && s.size <= core.MaxSuccessionSize:
			// Records are held in memory, so there is a limit to them
			// all: a zip file can state small sizes and unpack to more
			// than a machine has.
			if total += s.size; total > maxRecords {
				return Input{Fatal: []string{fmt.Sprintf("%s holds more records than can be checked here: more than %d MiB", name, maxRecords>>20)}}
			}
			data, err := readAll(s)
			if err != nil {
				notPlain[path[len(root):]] = true
				continue
			}
			in.Records[path[len(root):]] = data
		case parts[0] == "objects":
			// A file where a folder belongs, or a folder where a record does.
			notPlain[strings.Join(parts[:min(3, len(parts))], "/")] = true
		case parts[0] == "files" && len(parts) == 1:
			return Input{Fatal: []string{"files is not a folder: a link or a file in its place is not followed"}}
		case parts[0] == "files" && len(parts) == 2:
			files[parts[1]] = File{Name: parts[1], Regular: s.regular, Size: s.size, Digest: func() string {
				r, err := s.open()
				if err != nil {
					return ""
				}
				defer r.Close()
				return digestOf(r)
			}}
		case parts[0] == "files":
			files[parts[1]] = File{Name: parts[1], Size: -1, Digest: func() string { return "" }}
		default:
			ignored[parts[0]] = true
		}
	}
	for _, p := range slices.Sorted(maps.Keys(notPlain)) {
		in.Problems = append(in.Problems, fmt.Sprintf(notRecord, p))
	}
	in.Ignored = slices.Sorted(maps.Keys(ignored))
	for _, n := range slices.Sorted(maps.Keys(files)) {
		in.Files = append(in.Files, files[n])
	}
	return in
}

// readAll reads a source whose size is already known to be a record's, and
// fails when it does not hold what it states.
func readAll(s source) ([]byte, error) {
	r, err := s.open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err == nil && int64(len(data)) != s.size {
		err = errors.New("not the size it states")
	}
	return data, err
}
