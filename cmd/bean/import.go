package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Conscious-Repository/bean/core"
)

// bean import <source> [importer flags] [-corrections] <file> keeps the raw
// file first, then its readings. It runs bean-import-<source>, which never
// signs. Blob ids are deterministic, so this command knows the file's id
// before it stores anything. It checks the importer's first record, the one
// that keeps the file, and that the person did not purge it, before it
// stores the file. Then it stores the file, checks it reads back, appends
// that record, checks that the log keeps it, and only then appends the
// readings. Each step that can fail says what was written.

// maxImportFile bounds the private copy of a source file.
const maxImportFile = 256 << 20

// maxImporterOutput bounds what an importer writes: 1 GiB of readings, the
// Apple Health importer's own limit, and 1 MiB for the record of the file.
// The output goes to a private file, not to memory.
const maxImporterOutput = 1<<30 + 1<<20

// importerOutputLimit is maxImporterOutput; a test lowers it.
var importerOutputLimit int64 = maxImporterOutput

// maxImporterStderr is how much of the importer's stderr is kept: its last
// line is the message.
const maxImporterStderr = 64 << 10

var sourceName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func cmdImport(s *core.Store, args []string) {
	// -yes and -corrections are bean's own flags, wherever they are.
	var o importOpts
	var rest []string
	for _, a := range args {
		switch a {
		case "-yes", "--yes":
			o.yes = true
		case "-corrections", "--corrections":
			o.corrections = true
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) < 2 || !sourceName.MatchString(rest[0]) {
		usage()
	}
	source, file := rest[0], rest[len(rest)-1]
	pass := rest[1 : len(rest)-1]
	o.review = os.Stdin
	// The review is printed and answered: both ends must be a person's
	// terminal, or the readings would land in a pipe.
	o.terminal = isTerminal(os.Stdin) && isTerminal(os.Stdout)
	// Refused before the importer runs: its error for a line it cannot read
	// names that line, and the line can hold a value.
	if !o.yes && !o.terminal {
		fail(phaseError("review", errors.New("review needs a terminal: run bean import yourself to check what it read, or pass -yes to save without review")))
	}
	bin, err := importerPath(source)
	if err != nil {
		fail(err)
	}
	out, err := runImport(s, bin, file, pass, o)
	for _, l := range out.lines {
		fmt.Println(l)
	}
	if err != nil {
		fail(err)
	}
	if err := reportChanged(out.changed, "run bean import "+source+" -corrections"); err != nil {
		fail(fmt.Errorf("the file is kept. %w", err))
	}
}

// importerPath finds bean-import-<source> next to this binary, else on PATH.
func importerPath(source string) (string, error) {
	name := "bean-import-" + source
	if self, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(self), name)
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return p, nil
		}
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("no importer %s: build it with go install ./cmd/%s", name, name)
	}
	return p, nil
}

type importResult struct {
	lines   []string
	changed []core.Changed
}

// phase errors say what is on disk after a failure.
func phaseError(phase string, err error) error {
	msg := map[string]string{
		"validation": "the file was not accepted, and nothing was written",
		"purged":     "you deleted this file before (bean purge), and nothing was written",
		"review":     "nothing was written",
		"storage":    "the file could not be stored and read back; an encrypted copy may remain, but no record was written",
		"archive":    "the record that keeps the file was not added, and an encrypted copy of the file may remain; no reading was written",
		"retention":  "the log does not keep the file (it may have been deleted with bean purge); no reading was written",
		"readings":   "some readings may be written; run the same command again, it adds only what is missing",
	}[phase]
	return fmt.Errorf("%s: %w", msg, err)
}

// importOpts: corrections writes changed readings; yes saves without the
// review; review is where the person's answer comes from, and terminal says
// whether a person is there to give it.
type importOpts struct {
	corrections, yes, terminal bool
	review                     io.Reader
}

// maxReviewRows is how many readings the review lists; it counts the rest.
const maxReviewRows = 300

// reviewReadings prints what the importer read, one line per reading, and
// returns how many there are. rd is the importer's output after the record
// that keeps the file.
func reviewReadings(w io.Writer, rd *bufio.Reader) (int, error) {
	n := 0
	for {
		line, err := rd.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			n++
			if n <= maxReviewRows {
				fmt.Fprintln(w, "  "+safe(reviewLine(line)))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return n, err
		}
	}
	if n > maxReviewRows {
		fmt.Fprintf(w, "  ... and %d more\n", n-maxReviewRows)
	}
	return n, nil
}

// reviewLine is one reading as the review shows it: type, value, unit, flag.
func reviewLine(line []byte) string {
	var r struct {
		Type    string         `json:"type"`
		Value   map[string]any `json:"value"`
		Context map[string]any `json:"context"`
	}
	if json.Unmarshal(line, &r) != nil {
		return "(a line that is not a record)"
	}
	v := "no value"
	switch {
	case r.Value["n"] != nil:
		v = fmt.Sprint(r.Value["n"])
		if raw, ok := r.Context["raw_value"].(string); ok {
			v = raw
		}
	case r.Value["s"] != nil:
		v = fmt.Sprint(r.Value["s"])
	case r.Value["b"] != nil:
		v = fmt.Sprint(r.Value["b"])
	default:
		if c, _ := r.Context["lab_comment"].(string); strings.HasPrefix(strings.ToLower(c), "test not performed") {
			v = "not performed"
		}
	}
	if u, ok := r.Value["unit"].(string); ok && u != "" {
		v += " " + u
	}
	if f, ok := r.Context["flag"].(string); ok && f != "" {
		v += "  " + f
	}
	return r.Type + "  " + v
}

// confirmed reads the person's answer: y or yes saves, anything else does
// not.
func confirmed(r io.Reader) bool {
	line, _ := bufio.NewReader(r).ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(line))
	return a == "y" || a == "yes"
}

func runImport(s *core.Store, bin, file string, args []string, o importOpts) (importResult, error) {
	corrections := o.corrections
	var res importResult
	dir, err := os.MkdirTemp("", "bean-import-*")
	if err != nil {
		return res, err
	}
	// Deferred calls run last first: the folder goes, then the trap stops.
	// A signal between the two still finds the folder removed.
	defer removeOnSignal(dir)()
	defer os.RemoveAll(dir)
	// One private snapshot: the importer, the hash and the stored blob all see
	// the same bytes, whatever happens to the file meanwhile.
	snap := filepath.Join(dir, "source")
	data, err := snapshot(file, snap)
	if err != nil {
		return res, phaseError("validation", err)
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	bid, err := s.BlobIDOf(data)
	if err != nil {
		return res, phaseError("validation", err)
	}
	out, note, err := runImporter(bin, bid, args, snap, dir)
	if err != nil {
		return res, phaseError("validation", err)
	}
	defer out.Close()
	// Check the record that keeps the file before anything is written: an
	// importer that keeps no file, or a file the person deleted, leaves
	// nothing behind.
	rd := bufio.NewReaderSize(out, 64<<10)
	first, err := firstLine(rd)
	if err != nil {
		return res, phaseError("validation", err)
	}
	var archive struct {
		Type   string         `json:"type"`
		Value  map[string]any `json:"value"`
		Source map[string]any `json:"source"`
		Blobs  []struct {
			Hash string `json:"hash"`
		} `json:"blobs"`
	}
	listed := false
	if json.Unmarshal(first, &archive) == nil {
		for _, b := range archive.Blobs {
			listed = listed || b.Hash == bid
		}
	}
	if !listed || archive.Type == "" || archive.Value["blob"] != bid {
		return res, phaseError("validation", errors.New("the importer did not start with a record of the file"))
	}
	// The purge check computes the ref from ref_input. A ready-made ref
	// cannot be checked under another system, so refuse it.
	if _, ok := archive.Source["ref_input"].([]any); !ok {
		return res, phaseError("validation", errors.New("the record of the file has no source.ref_input"))
	}
	if _, has := archive.Source["ref"]; has {
		return res, phaseError("validation", errors.New("the record of the file carries a ready-made source.ref"))
	}
	// Append skips a line whose supersedes_ref names nothing: the file would
	// be stored with no record naming it.
	if _, has := archive.Source["supersedes_ref"]; has {
		return res, phaseError("validation", errors.New("the record of the file carries source.supersedes_ref"))
	}
	purged, err := s.InputPurged(first)
	if err != nil {
		return res, phaseError("validation", err)
	}
	if purged {
		return res, phaseError("purged", errors.New(archive.Type+" "+bid))
	}
	// Append's own checks of the record (its fields, its size, the key) run
	// now too. Once the file is stored, a refusal would leave it there with
	// no record naming it, and gc erases only what a record named.
	if err := s.CheckPartial(first); err != nil {
		return res, phaseError("validation", err)
	}
	// The person checks what was read before anything is written. Without a
	// terminal and without -yes, nothing is shown: the readings would land
	// in a script's log or an AI assistant's context.
	if !o.yes {
		if !o.terminal {
			return res, phaseError("review", errors.New("review needs a terminal: run bean import yourself to check what it read, or pass -yes to save without review"))
		}
		if _, err := out.Seek(int64(len(first))+1, io.SeekStart); err != nil {
			return res, phaseError("validation", err)
		}
		fmt.Println("Read from " + filepath.Base(file) + ":")
		n, err := reviewReadings(os.Stdout, bufio.NewReader(out))
		if err != nil {
			return res, phaseError("validation", err)
		}
		fmt.Fprintf(os.Stderr, "Save %d readings and keep the file? [y/N] ", n)
		if !confirmed(o.review) {
			return res, phaseError("review", errors.New("not saved"))
		}
		if _, err := out.Seek(int64(len(first))+1, io.SeekStart); err != nil {
			return res, phaseError("validation", err)
		}
		rd = bufio.NewReaderSize(out, 64<<10)
	}
	stored, err := s.PutBlob(data)
	if err != nil {
		return res, phaseError("storage", err)
	}
	back, err := s.GetBlob(bid)
	if stored != bid || err != nil || !bytes.Equal(back, data) {
		return res, phaseError("storage", errors.New("the stored copy does not read back the same"))
	}
	a, err := s.Append(bytes.NewReader(append(first, '\n')))
	if err != nil {
		return res, phaseError("archive", err)
	}
	rid, err := kept(s, archive.Type, bid, digest)
	if err != nil {
		return res, phaseError("retention", err)
	}
	// The record id is what bean purge takes to erase the file; the file's
	// SHA-256 is what shasum gives; the blob id names the encrypted copy.
	what := fmt.Sprintf("%s %s: file sha256 %s, stored as %s", archive.Type, rid, digest, bid)
	if a.Added == 1 {
		res.lines = append(res.lines, "kept "+what)
	} else {
		res.lines = append(res.lines, "already kept "+what)
	}
	appendFn := s.Append
	if corrections {
		appendFn = s.AppendCorrections
	}
	r, err := appendFn(rd)
	line := fmt.Sprintf("added %d, skipped %d", r.Added, r.Skipped)
	if r.Corrected > 0 {
		line += fmt.Sprintf(", %d of them new values for readings you already had", r.Corrected)
	}
	res.lines = append(res.lines, line)
	res.changed = r.Changed
	if err != nil {
		return res, phaseError("readings", err)
	}
	if _, err := kept(s, archive.Type, bid, digest); err != nil {
		return res, phaseError("retention", err)
	}
	// The importer's own last line comes only now, made safe and named, so
	// it can neither fake a line of bean's nor speak for a failed import.
	if note != "" {
		name := filepath.Base(bin)
		if note = safe(note); !strings.HasPrefix(note, name+": ") {
			note = name + ": " + note
		}
		res.lines = append(res.lines, note)
	}
	return res, nil
}

// firstLine reads the importer's first line, at most one record long.
func firstLine(rd *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, err := rd.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > core.MaxRecordSize {
			return nil, errors.New("the importer's first record is too large")
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		return bytes.TrimSuffix(line, []byte("\n")), nil
	}
}

// running is the importer process, so a signal can stop it too.
var running struct {
	sync.Mutex
	p *os.Process
}

// importSignals stop bean import: Ctrl-C, kill, a closed terminal or ssh
// session, and Ctrl-\.
var importSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT}

// removeOnSignal removes the private folder, which holds a plaintext copy of
// the file, when bean import gets one of importSignals. A deferred RemoveAll
// does not run then. It exits 128 plus the signal number, as a shell does.
// The returned function stops the trap.
func removeOnSignal(dir string) func() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, importSignals...)
	done := make(chan struct{})
	go func() {
		select {
		case got := <-sig:
			running.Lock()
			if running.p != nil {
				// The importer leads its own process group, so pdftotext and any
				// other child it started die with it, also after the importer
				// itself has exited and Wait has reaped it: a child that keeps
				// its output open holds Wait. The group id stays taken while the
				// group has a member, so the kill reaches no other process.
				syscall.Kill(-running.p.Pid, syscall.SIGKILL)
				running.p.Kill()
			}
			os.RemoveAll(dir)
			fmt.Fprintf(os.Stderr, "bean: stopped by %s; the private copy of the file is removed. Run the same command again: it adds only what is missing\n", got)
			code := 1
			if n, ok := got.(syscall.Signal); ok {
				code = 128 + int(n)
			}
			os.Exit(code)
		case <-done:
		}
	}()
	return func() {
		signal.Stop(sig)
		close(done)
	}
}

// snapshot copies a regular file, at most maxImportFile bytes, and returns it.
func snapshot(src, dst string) ([]byte, error) {
	info, err := os.Lstat(src)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file: " + src)
	}
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if opened, err := f.Stat(); err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("the file changed while it was opened")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxImportFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImportFile {
		return nil, fmt.Errorf("the file is larger than %d MiB", maxImportFile>>20)
	}
	return data, os.WriteFile(dst, data, 0o600)
}

// runImporter runs the importer with its temporary files in dir, writes its
// output to a private file in dir, at most maxImporterOutput bytes, and
// returns that file at its start and the last line it wrote to stderr.
func runImporter(bin, blob string, args []string, file, dir string) (*os.File, string, error) {
	f, err := os.OpenFile(filepath.Join(dir, "out"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, "", err
	}
	cmd := exec.Command(bin, append(append([]string{"-blob", blob}, args...), file)...)
	// The importer's own temporary copies land in the private folder too.
	cmd.Env = append(os.Environ(), "TMPDIR="+dir)
	// Its own process group: a signal to bean stops the whole group, and
	// Ctrl-C at the terminal reaches only bean, which then stops it. On
	// Linux the kernel also kills it when bean dies (importerAttr).
	cmd.SysProcAttr = importerAttr()
	// A process the importer started may keep its output open after the
	// importer exits. Wait gives up after a second, and the import fails.
	cmd.WaitDelay = time.Second
	stdout := &capped{w: f, left: importerOutputLimit}
	stderr := &tail{max: maxImporterStderr}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	// The parent-death signal follows the thread that started the importer:
	// keep this goroutine on it until the importer is done.
	runtime.LockOSThread()
	running.Lock()
	err = cmd.Start()
	if err == nil {
		running.p = cmd.Process
	}
	running.Unlock()
	if err == nil {
		err = cmd.Wait()
	}
	running.Lock()
	if errors.Is(err, exec.ErrWaitDelay) {
		// A member of the group still holds the output, so the group id is
		// still taken: stop it.
		syscall.Kill(-running.p.Pid, syscall.SIGKILL)
	}
	running.p = nil
	running.Unlock()
	runtime.UnlockOSThread()
	msg := lastLine(string(stderr.b))
	if errors.Is(err, exec.ErrWaitDelay) {
		msg = "the importer exited, but a process it started kept its output open"
	}
	if stdout.over {
		f.Close()
		return nil, "", fmt.Errorf("the importer wrote more than %d MiB", importerOutputLimit>>20)
	}
	if err != nil {
		f.Close()
		if msg == "" {
			msg = err.Error()
		}
		return nil, "", errors.New(msg)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, "", err
	}
	return f, msg, nil
}

// capped writes at most left bytes, then fails, which stops the importer.
type capped struct {
	w    io.Writer
	left int64
	over bool
}

func (c *capped) Write(p []byte) (int, error) {
	if int64(len(p)) > c.left {
		c.over = true
		return 0, errors.New("importer output exceeds limit")
	}
	n, err := c.w.Write(p)
	c.left -= int64(n)
	return n, err
}

// tail keeps the last max bytes written to it.
type tail struct {
	b   []byte
	max int
}

func (t *tail) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > t.max {
		t.b = append(t.b[:0], t.b[len(t.b)-t.max:]...)
	}
	return len(p), nil
}

func lastLine(s string) string {
	var last string
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		if t := strings.TrimSpace(sc.Text()); t != "" && !strings.Contains(t, "warning:") {
			last = t
		}
	}
	return last
}

// kept reports whether the log keeps the person's record of this exact
// file, and the file itself. It proves every record file, because the object
// cache vouches for a file by its size and time, and it reads the blob back.
// It returns the id of that record, the one bean purge takes to erase the
// file.
func kept(s *core.Store, typ, bid, digest string) (string, error) {
	l, err := s.LoadFull()
	if err != nil {
		return "", err
	}
	if l.IsTooNew() {
		return "", l.TooNewError()
	}
	found := ""
	for _, id := range slices.Sorted(maps.Keys(l.Report.Records)) {
		r := l.Report.Records[id]
		if found != "" || r.Type != typ || !l.Current(id) || r.Author != l.ID || r.Subject != l.ID || r.Value["blob"] != bid || r.Context["sha256"] != digest {
			continue
		}
		for _, b := range r.Blobs {
			if b.Hash == bid {
				found = id
			}
		}
	}
	if found == "" {
		return "", errors.New("no current record of yours keeps this file")
	}
	data, err := s.GetBlob(bid)
	if err != nil {
		return "", err
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != digest {
		return "", errors.New("the stored copy no longer matches the file")
	}
	return found, nil
}
