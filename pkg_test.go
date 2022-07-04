package zipserver_test

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

const testFileName = "LICENSE.txt"

func zipFile() *zip.Reader {
	buf := new(bytes.Buffer)
	b, err := os.ReadFile(testFileName)
	if err != nil {
		panic(err)
	}
	zw := zip.NewWriter(buf)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: testFileName, Method: zip.Deflate})
	if err != nil {
		panic(err)
	}
	if _, err := w.Write(b); err != nil {
		panic(err)
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
