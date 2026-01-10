package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var assets embed.FS

// GetFileSystem returns the embedded frontend assets as an http.FileSystem
func GetFileSystem(useEmbedded bool) http.FileSystem {
	if useEmbedded {
		// "dist" ディレクトリをルートとして公開
		f, _ := fs.Sub(assets, "dist")
		return http.FS(f)
	}
	// 開発用：ローカルの web ディレクトリを直接参照
	return http.Dir("web")
}
