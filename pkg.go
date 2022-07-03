// Package zipserver provides a helper function to serve static assets over HTTP directly from a ZIP file.
//
// Its main advantage over [zip.Reader] wrapped with [http.FS] and [http.FileServer] is ability to directly serve Deflate-compressed contents insize ZIP archives to clients.
//
// Usage example:
//
//	func run(filename string) error {
//		zr, err := zip.OpenReader(filename)
//		if err != nil {
//			return err
//		}
//		defer zr.Close()
//		return http.ListenAndServe("localhost:8000", zipserver.Handler(&zr.Reader))
//	}
package zipserver

import (
	"archive/zip"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// Handler wraps *zip.Reader, providing HTTP access to its contents.
// If an incoming HTTP request announces support for compressed content with “Accept-Encoding: deflate” header, and a requested file inside a ZIP archive is compressed with Deflate method, Handler serves such file to the client as a “Content-Encoding: deflate” response.
func Handler(z *zip.Reader) http.Handler {
	// deflate-compressed files, name to index in z.File
	m := make(map[string]int)
	srv := http.FileServer(http.FS(z))
	for i := range z.File {
		if z.File[i].Method != zip.Deflate {
			continue
		}
		m[z.File[i].Name] = i
	}
	if len(m) == 0 {
		return srv
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		w.Header().Set("Accept-Ranges", "none")

		if r.Method != http.MethodGet ||
			r.Header.Get("Range") != "" ||
			!strings.Contains(r.Header.Get("Accept-Encoding"), "deflate") {
			srv.ServeHTTP(w, r)
			return
		}

		key := strings.TrimPrefix(r.URL.Path, "/")
		if key == "" {
			key = "index.html"
		}
		i, ok := m[key]
		if !ok {
			srv.ServeHTTP(w, r)
			return
		}

		rd, err := z.File[i].OpenRaw()
		if err != nil {
			srv.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Type", conjureContentType(z.File[i]))
		w.Header().Set("Content-Length", strconv.FormatUint(z.File[i].CompressedSize64, 10))
		w.Header().Set("Content-Encoding", "deflate")
		io.Copy(w, rd)
	})
}

func conjureContentType(zf *zip.File) string {
	if s := mime.TypeByExtension(path.Ext(zf.Name)); s != "" {
		return s
	}
	rc, err := zf.Open()
	if err != nil {
		return "application/octet-stream"
	}
	defer rc.Close()
	b := make([]byte, 512)
	i, _ := rc.Read(b)
	return http.DetectContentType(b[:i])
}
