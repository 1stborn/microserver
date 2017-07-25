package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"compress/gzip"
	"regexp"
)

type Config struct {
	Httpd struct {
		Listen    string
		Hostname  string
		Root      string
		AccessLog string
		Gzip      []string
	}
}

var config *Config

func init() {
	if f, err := os.OpenFile("config.json", 0, 0); err == nil {
		config = new(Config)
		if err := json.NewDecoder(f).Decode(config); err != nil {
			panic(err)
		}
	}
}

type Handler struct {
	files http.Handler
}

type Log struct {
	io.Writer

	Path    string
	Handler http.Handler
}

type GZip struct {
	*regexp.Regexp
	http.Handler
}

type Writer struct {
	http.ResponseWriter

	Status int
	Bytes  int
}

type GzipWriter struct {
	gz io.Writer
	http.ResponseWriter
}

func main() {
	var writer io.Writer

	if f, err := os.OpenFile(config.Httpd.AccessLog, os.O_CREATE|os.O_APPEND, 0); err == nil {
		writer = f
	} else {
		writer = os.Stdout
	}

	http.ListenAndServe(config.Httpd.Listen, Log{
		Writer: writer,
		Path:   config.Httpd.AccessLog,
		Handler: GZip{
			regexp.MustCompile("(?i)\\.(" + strings.Join(config.Httpd.Gzip, "|") + ")$"),
			Handler{
				http.FileServer(http.Dir(config.Httpd.Root)),
			},
		},
	})
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.Index(r.Host, config.Httpd.Hostname) != -1 {
		h.files.ServeHTTP(w, r)
	} else {
		http.NotFound(w, r)
	}
}

func (h GZip) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		uri := r.RequestURI

		if uri == "/" {
			uri += "index.html"
		}

		if h.Match([]byte(uri)) {
			w.Header().Set("Content-Encoding", "gzip")

			gz := gzip.NewWriter(w)
			defer gz.Close()

			h.Handler.ServeHTTP(GzipWriter{gz: gz, ResponseWriter: w}, r)

			return
		}
	}

	h.Handler.ServeHTTP(w, r)
}

func (l Log) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lw := Writer{ResponseWriter: w}

	defer func() {
		var addr string

		if idx := strings.LastIndex(r.RemoteAddr, ":"); idx > 0 {
			addr = r.RemoteAddr[:idx]
		} else {
			addr = r.RemoteAddr
		}

		addr = strings.Trim(addr, "[]")

		fmt.Fprintf(l, "%s [%s] %s \"%s\" %d %d \"%s\" \"%s\"\n",
			addr,
			time.Now().Format("02/Jan/2006 15:04:05 -0700"),
			r.Method,
			r.RequestURI,
			lw.Status,
			lw.Bytes,
			r.Header.Get("referer"),
			r.UserAgent())
	}()

	l.Handler.ServeHTTP(&lw, r)
}

func (w GzipWriter) Write(b []byte) (int, error) {
	return w.gz.Write(b)
}

func (w *Writer) Write(b []byte) (int, error) {
	w.Bytes += len(b)
	return w.ResponseWriter.Write(b)
}

func (w *Writer) WriteHeader(s int) {
	w.Status = s
	w.ResponseWriter.WriteHeader(s)
}
