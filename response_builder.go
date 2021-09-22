package viewproxy

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strconv"

	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
)

type responseBuilder struct {
	writer     http.ResponseWriter
	server     Server
	body       []byte
	StatusCode int
}

func newResponseBuilder(server Server, w http.ResponseWriter) *responseBuilder {
	return &responseBuilder{server: server, writer: w, StatusCode: 200}
}

func (rb *responseBuilder) SetLayout(result *multiplexer.Result) {
	rb.body = result.Body
}

func (rb *responseBuilder) SetFragments(results []*multiplexer.Result) {
	var contentHtml []byte

	for _, result := range results {
		contentHtml = append(contentHtml, result.Body...)
	}

	if len(rb.body) == 0 {
		rb.body = contentHtml
	} else {
		outputHtml := bytes.Replace(rb.body, []byte("<view-proxy-content></view-proxy-content>"), contentHtml, 1)

		rb.body = outputHtml
	}
}

func (rb *responseBuilder) SetDuration(duration int64) {
	outputHtml := bytes.Replace(rb.body, []byte("<view-proxy-timing></view-proxy-timing>"), []byte(strconv.FormatInt(duration, 10)), 1)
	rb.body = outputHtml
}

func (rb *responseBuilder) Write() {
	rb.writer.WriteHeader(rb.StatusCode)

	if rb.writer.Header().Get("Content-Encoding") == "gzip" {
		var b bytes.Buffer
		gzipWriter := gzip.NewWriter(&b)

		_, err := gzipWriter.Write(rb.body)
		if err != nil {
			rb.server.Logger.Printf("Could not write to gzip buffer: %s", err)
		}

		gzipWriter.Close()
		if err != nil {
			rb.server.Logger.Printf("Could not closeto gzip buffer: %s", err)
		}

		rb.writer.Write(b.Bytes())
	} else {
		rb.writer.Write(rb.body)
	}
}
