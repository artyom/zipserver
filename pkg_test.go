package zipserver_test

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"

	"artyom.dev/zipserver"
)

func TestHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://localhost/"+testFileName, nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	w := httptest.NewRecorder()
	zipserver.Handler(zipFile()).ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %s", resp.Status)
	}
	if s := resp.Header.Get("Vary"); s != "Accept-Encoding" {
		t.Fatalf("unexpected Vary value: %q", s)
	}
	if s := resp.Header.Get("Content-Encoding"); s != "deflate" {
		t.Fatalf("unexpected Content-Encoding value (want deflate): %q", s)
	}
	if s := resp.Header.Get("Content-Type"); s != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected Content-Type value: %q", s)
	}
	fr := flate.NewReader(resp.Body)
	defer fr.Close()
	got, err := io.ReadAll(fr)
	if err != nil {
		t.Fatalf("decompressing body: %v", err)
	}
	want, err := os.ReadFile(testFileName)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("payloads differ (got %d bytes, want %d bytes)", len(got), len(want))
	}
}

func TestHandler_seekableFile(t *testing.T) {
	if ct := mime.TypeByExtension(path.Ext(testFileNoSuffix)); ct != "" {
		t.Fatalf("got non-empty mime type for file named %q: %q", testFileNoSuffix, ct)
	}
	req := httptest.NewRequest(http.MethodGet, "http://localhost/"+testFileNoSuffix, nil)
	w := httptest.NewRecorder()
	zipserver.Handler(zipFile()).ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %s", resp.Status)
	}
	if s := resp.Header.Get("Vary"); s != "Accept-Encoding" {
		t.Fatalf("unexpected Vary value: %q", s)
	}
	if s := resp.Header.Get("Content-Type"); s != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected Content-Type value: %q", s)
	}
	if resp.Uncompressed {
		t.Fatal("got automatically uncompressed response")
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	want, err := os.ReadFile(testFileName)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("payloads differ (got %d bytes, want %d bytes)", len(got), len(want))
	}
}

func BenchmarkHandler(b *testing.B) {
	handler := zipserver.Handler(zipFile())
	req := httptest.NewRequest(http.MethodGet, "http://localhost/"+testFileName, nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status: %s", resp.Status)
		}
		io.Copy(io.Discard, resp.Body)
	}
}

const testFileName = "LICENSE.txt"
const testFileNoSuffix = "unknown"

func zipFile() *zip.Reader {
	buf := new(bytes.Buffer)
	b, err := os.ReadFile(testFileName)
	if err != nil {
		panic(err)
	}
	zw := zip.NewWriter(buf)
	for _, name := range [...]string{testFileName, testFileNoSuffix} {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			panic(err)
		}
		if _, err := w.Write(b); err != nil {
			panic(err)
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	rd := bytes.NewReader(buf.Bytes())
	zr, err := zip.NewReader(rd, rd.Size())
	if err != nil {
		panic(err)
	}
	return zr
}
