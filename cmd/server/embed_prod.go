//go:build embedweb

package main

import (
	"embed"
	"io/fs"
)

//go:embed all:webdist
var webDistFS embed.FS

func webFS() fs.FS {
	sub, err := fs.Sub(webDistFS, "webdist")
	if err != nil {
		panic(err)
	}
	return sub
}