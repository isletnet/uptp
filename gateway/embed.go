package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web
var webFS embed.FS

// 获取嵌入的 web 文件系统
func getWebFS() http.FileSystem {
	fsys, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	return http.FS(fsys)
}
