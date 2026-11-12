//go:build js && wasm

// Command check is the bundle check of bean check-bundle, built for a
// browser. tools/page/build.go puts it into one page, bean-check.html. It
// gives the page one function and reads nothing but what the page hands it.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/Conscious-Repository/bean/internal/bundle"
	"github.com/Conscious-Repository/bean/internal/view"
)

// carried holds the files of the last bundle that passed, by SHA-256, for
// a reader who wants to save one.
var carried = map[string]func() ([]byte, error){}

func main() {
	js.Global().Set("beanCheck", js.FuncOf(check))
	js.Global().Set("beanFile", js.FuncOf(file))
	select {}
}

// file is beanFile(sha256): the bytes of a file that the last passed
// bundle carries, or null. The bytes are hashed once more, so that what
// is saved is what was checked.
func file(_ js.Value, args []js.Value) (out any) {
	defer func() {
		if recover() != nil {
			out = js.Null()
		}
	}()
	read := carried[args[0].String()]
	if read == nil {
		return js.Null()
	}
	data, err := read()
	if sum := sha256.Sum256(data); err != nil || hex.EncodeToString(sum[:]) != args[0].String() {
		return js.Null()
	}
	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	return arr
}

// checked runs the check, and keeps the files of a bundle that passed.
func checked(in bundle.Input) bundle.Result {
	res := bundle.Check(in)
	carried = map[string]func() ([]byte, error){}
	if res.OK {
		for _, f := range in.Files {
			carried[f.Name] = f.Read
		}
	}
	return res
}

// check is beanCheck(name, file, paths, datas, words). file is the bytes of
// a zip file or of a sealed package, or null; then paths and datas are the
// files of a chosen folder. words open a sealed package. It
// returns the result as JSON: bundle.Result, and lines, the same text that
// bean check-bundle prints. Every string is made safe to show.
func check(_ js.Value, args []js.Value) (out any) {
	// The check fails closed: whatever goes wrong in it, the page is told
	// that the bundle did not pass.
	defer func() {
		if r := recover(); r != nil {
			out = encode(bundle.Check(bundle.Input{Fatal: []string{fmt.Sprintf("the check itself failed, so nothing is shown: %v", r)}}))
		}
	}()
	name := args[0].String()
	if !args[1].IsNull() {
		words := ""
		if len(args) > 4 && args[4].Type() == js.TypeString {
			words = args[4].String()
		}
		return encode(checked(bundle.FromPackage(name, bytesOf(args[1]), words)))
	}
	files := map[string][]byte{}
	for i := range args[2].Length() {
		path := args[2].Index(i).String()
		if _, twice := files[path]; twice {
			panic("two files with one name")
		}
		files[path] = bytesOf(args[3].Index(i))
	}
	return encode(checked(bundle.FromFiles(name, files)))
}

func bytesOf(v js.Value) []byte {
	b := make([]byte, v.Length())
	js.CopyBytesToGo(b, v)
	return b
}

func encode(res bundle.Result) string {
	lines := res.Lines()
	safe := func(list []string) []string {
		out := make([]string, len(list))
		for i, s := range list {
			out[i] = view.Safe(s)
		}
		return out
	}
	shown := struct {
		bundle.Result
		Lines []string `json:"lines"`
	}{res, safe(lines)}
	shown.Head, shown.Tail, shown.Problems, shown.Ignored, shown.Note = safe(res.Head), safe(res.Tail), safe(res.Problems), safe(res.Ignored), view.Safe(res.Note)
	if res.Rows != nil {
		shown.Rows = make([]bundle.Row, len(res.Rows))
		for i, r := range res.Rows {
			shown.Rows[i] = bundle.Row{When: view.Safe(r.When), Type: view.Safe(r.Type), Text: view.Safe(r.Text), By: view.Safe(r.By), ID: r.ID, Mark: r.Mark}
		}
	}
	if res.Carried != nil {
		shown.Carried = make([]bundle.Carried, len(res.Carried))
		for i, c := range res.Carried {
			c.Type, c.Media = view.Safe(c.Type), view.Safe(c.Media)
			shown.Carried[i] = c
		}
	}
	data, err := json.Marshal(shown)
	if err != nil {
		panic(err)
	}
	return string(data)
}
