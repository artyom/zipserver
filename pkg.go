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
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
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

	// when content-type cannot be derived from the file name, http.serveContent
	// reads a small buffer from the file to sniff the content type, and then tries
	// to seek back to the start. zip.Reader files don't support seeking, so route
	// all requests for which the content type cannot be detected purely from the
	// file name to a fallback FS implementation, which pretends its files to be
	// seekable (only to the beginnig of the file) by re-opening the file.
	srvSeek0 := http.FileServer(http.FS(seekableFS{z}))
	fallbackServe := func(w http.ResponseWriter, r *http.Request) {
		if mime.TypeByExtension(path.Ext(r.URL.Path)) == "" {
			srvSeek0.ServeHTTP(w, r)
			return
		}
		srv.ServeHTTP(w, r)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		w.Header().Set("Accept-Ranges", "none")

		if r.Method != http.MethodGet ||
			r.Header.Get("Range") != "" ||
			!strings.Contains(r.Header.Get("Accept-Encoding"), "deflate") {
			fallbackServe(w, r)
			return
		}

		key := strings.TrimPrefix(r.URL.Path, "/")
		if key == "" {
			key = "index.html"
		}
		i, ok := m[key]
		if !ok {
			fallbackServe(w, r)
			return
		}

		rd, err := z.File[i].OpenRaw()
		if err != nil {
			fallbackServe(w, r)
			return
		}

		w.Header().Set("Content-Type", conjureContentType(z.File[i]))
		w.Header().Set("Content-Length", strconv.FormatUint(z.File[i].CompressedSize64, 10))
		w.Header().Set("Content-Encoding", "deflate")
		w.Header().Set("Last-Modified", z.File[i].Modified.UTC().Format(http.TimeFormat))
		b := bufPool.Get().(*[]byte)
		io.CopyBuffer(w, rd, *b)
		bufPool.Put(b)
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

var bufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 32*1024)
		return &b
	},
}

type seekableFS struct{ *zip.Reader }

func (s seekableFS) Open(name string) (fs.File, error) {
	file, err := s.Reader.Open(name)
	if err != nil {
		return nil, err
	}
	return &seekableFile{File: file, zr: s.Reader, name: name}, nil
}

// A seekableFile wraps a fs.File returned by zip.Reader.
// This wrapper mimics a Seek method by re-opening a wrapped file,
// which tricks callers expecting a working seek to the beginning of the file.
type seekableFile struct {
	fs.File
	zr   *zip.Reader
	name string
}

func (f *seekableFile) Seek(offset int64, whence int) (int64, error) {
	if offset != 0 || whence != io.SeekStart {
		return 0, errors.New("seekableFile does not support arbitrary seeks")
	}
	if err := f.File.Close(); err != nil {
		return 0, err
	}
	file, err := f.zr.Open(f.name)
	if err != nil {
		return 0, err
	}
	f.File = file
	return 0, nil
}
