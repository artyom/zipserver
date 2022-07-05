// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// code below is a copy from https://github.com/golang/website/blob/ecd2fa3d3c8c32c635d71aa36fcb956d50270df6/cmd/golangorg/server.go#L694-L736

package memseek

import (
	"bytes"
	"io"
	"io/fs"
)

func FS(f fs.FS) *seekableFS { return &seekableFS{f} }

// A seekableFS is an FS wrapper that makes every file seekable
// by reading it entirely into memory when it is opened and then
// serving read operations (including seek) from the memory copy.
type seekableFS struct {
	fs fs.FS
}

func (s *seekableFS) Open(name string) (fs.File, error) {
	f, err := s.fs.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if info.IsDir() {
		return f, nil
	}
	data := make([]byte, int(info.Size()))
	if _, err := io.ReadFull(f, data); err != nil {
		f.Close()
		return nil, err
	}
	var sf seekableFile
	sf.File = f
	sf.Reset(data)
	return &sf, nil
}

// A seekableFile is a fs.File augmented by an in-memory copy of the file data to allow use of Seek.
type seekableFile struct {
	bytes.Reader
	fs.File
}

// Read calls f.Reader.Read.
// Both f.Reader and f.File have Read methods - a conflict - so f inherits neither.
// This method calls the one we want.
func (f *seekableFile) Read(b []byte) (int, error) {
	return f.Reader.Read(b)
}
