package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings" // Added
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
	// 開発用：ローカルの web/dist ディレクトリを直接参照
	return SpaFileSystem{http.Dir("web/dist")}
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

	// If the file is not found and the request is for an asset (starts with /assets/),
	// then it's a genuine 404 for the asset. Do not fall back to index.html.
	if strings.HasPrefix(name, "/assets/") {
		return nil, err // Return the original error
	}

	// For all other paths (potential client-side routes), fall back to index.html (SPA routing)
	f, err = sfs.base.Open("index.html")
	if err != nil {
		return nil, err
	}

	return f, nil
}
