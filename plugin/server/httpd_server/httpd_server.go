/*
PaoPaoDNS httpd server
*/

package httpd_server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/IrineSistiana/mosdns/v5/coremain"
)

func init() {
	coremain.RegNewPluginFunc("httpd_server", Init, func() any { return new(struct{}) })
}

func Init(bp *coremain.BP, args any) (any, error) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		dirPath := "/data"
		filePath := filepath.Join(dirPath, path)
		fi, err := os.Stat(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if fi.IsDir() {
			fileList, err := os.ReadDir(filePath)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			var sb strings.Builder
			sb.WriteString("<!DOCTYPE html><html><head><title>PaoPaoDNS:/Data</title><style>li{font-size:30px}</style><meta content=\"text/html; charset=utf-8\" http-equiv=\"content-type\" /></head><body><h2>PaoPaoDNS:/Data</h2><hr><ul>")
			for _, file := range fileList {
				fileName := file.Name()
				fileLink := filepath.Join(path, fileName)
				sb.WriteString(fmt.Sprintf("<li><a href=\"%s\">%s</a></li>", fileLink, fileName))
			}
			sb.WriteString("</ul><hr><a href=https://github.com/kkkgo/PaoPaoDNS/discussions>https://github.com/kkkgo/PaoPaoDNS</a></body></html>")
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, sb.String())
		} else {
			http.ServeFile(w, r, filePath)
		}
	})

	fmt.Println("httpd on port 7889...")
	err := http.ListenAndServe(":7889", nil)
	if err != nil {
		fmt.Println("Server error:", err)
	}
	return nil, nil
}
