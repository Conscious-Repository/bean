package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// '>' is legal in an attribute value, so it resets tagBound's run while
// encoding/xml keeps building one element's attributes.
func TestA13AttrGtBypassesTagBound(t *testing.T) {
	var x bytes.Buffer
	x.WriteString(`<?xml version="1.0" encoding="UTF-8"?><HealthData><Me`)
	for i := 0; i < 3_000_000; i++ {
		fmt.Fprintf(&x, ` a%x=">"`, i)
	}
	x.WriteString(`/></HealthData>`)
	var z bytes.Buffer
	zw := zip.NewWriter(&z)
	w, _ := zw.Create("apple_health_export/export.xml")
	w.Write(x.Bytes())
	zw.Close()
	p := filepath.Join(t.TempDir(), "export.zip")
	os.WriteFile(p, z.Bytes(), 0o600)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	err := convertFile(p, io.Discard, options{blob: testBlob})
	runtime.ReadMemStats(&after)
	t.Logf("zip %d B, xml %d B, err=%v, allocated %d MiB", z.Len(), x.Len(), err, (after.TotalAlloc-before.TotalAlloc)>>20)
	if after.TotalAlloc-before.TotalAlloc > 64<<20 {
		t.Errorf("allocated %d MiB before the refusal", (after.TotalAlloc-before.TotalAlloc)>>20)
	}
}
