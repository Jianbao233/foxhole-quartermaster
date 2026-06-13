//go:build !embedweb

package main

import (
	"io/fs"

	"github.com/Jianbao233/foxhole-quartermaster/internal/server"
)

// 开发态：不嵌入 web/dist，server 自动回退到 dev 占位页 + 提示去用 vite。
func webFS() fs.FS {
	return server.NoEmbedFS()
}