package handler

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// 文件存在时直接提供；仅浏览器 HTML 导航回退 SPA，资源缺失保持 404。
func Static(files fs.FS) http.Handler {
	server := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if file == "" {
			file = "index.html"
		}
		if _, err := fs.Stat(files, file); err != nil && strings.Contains(r.Header.Get("Accept"), "text/html") && path.Ext(file) == "" {
			clone := r.Clone(r.Context())
			clone.URL.Path = "/"
			server.ServeHTTP(w, clone)
			return
		}
		server.ServeHTTP(w, r)
	})
}
