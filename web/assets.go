package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var assets embed.FS

// GetFileSystem returns the embedded frontend assets as an http.FileSystem
func GetFileSystem(useEmbedded bool) http.FileSystem {
	if useEmbedded {
		// "dist" ディレクトリをルートとして公開
		f, err := fs.Sub(assets, "dist")
		if err != nil {
			// フォールバック：空のファイルシステムを返すか、エラーをログ出力
			return http.Dir("web/dist")
		}
		return SpaFileSystem{http.FS(f)}
	}
	// 開発用：ローカルの web ディレクトリを直接参照
	return http.Dir("web")
}

// SpaFileSystem wraps a FileSystem to support SPA routing
type SpaFileSystem struct {
	base http.FileSystem
}

func (sfs SpaFileSystem) Open(name string) (http.File, error) {
	f, err := sfs.base.Open(name)
	if err == nil {
		return f, nil
	}

	// ファイルが見つからない場合は index.html を試す (SPA routing)
	f, err = sfs.base.Open("index.html")
	if err != nil {
		return nil, err
	}

	return f, nil
}
