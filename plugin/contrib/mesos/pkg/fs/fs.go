/*
Copyright 2015 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fs

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
)

// ZipWalker returns a filepath.WalkFunc that adds every filesystem node
// to the given *zip.Writer.
func ZipWalker(zw *zip.Writer) filepath.WalkFunc {
	var base string
	return func(path string, info os.FileInfo, err error) error {
		if base == "" {
			base = path
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		if header.Name, err = filepath.Rel(base, path); err != nil {
			return err
		} else if info.IsDir() {
			header.Name = header.Name + string(filepath.Separator)
		} else {
			header.Method = zip.Deflate
		}

		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}

		_, err = io.Copy(w, f)
		f.Close()
		return err
	}
}
