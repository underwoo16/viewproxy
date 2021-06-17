package viewproxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasicServer(t *testing.T) {
	targetServer := startTargetServer()
	defer targetServer.Shutdown(context.TODO())

	viewProxyServer := NewServer("http://localhost:9994")
	viewProxyServer.Port = 9998
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	viewProxyServer.IgnoreHeader("etag")
	layout := NewFragment("/layouts/test_layout")
	fragments := []*Fragment{
		NewFragment("header"),
		NewFragment("body"),
		NewFragment("footer"),
	}
	viewProxyServer.Get("/hello/:name", layout, fragments)

	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	defer viewProxyServer.Close()

	resp, err := http.Get("http://localhost:9998/hello/world")

	if err != nil {
		t.Fatal(err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	expected := "<html><body>hello world</body></html>"

	assert.Equal(t, expected, string(body))
	assert.Equal(t, "viewproxy", resp.Header.Get("x-name"), "Expected response to have an X-Name header")
	assert.Equal(t, "", resp.Header.Get("etag"), "Expected response to have removed etag header")
}

func TestServerFromConfig(t *testing.T) {
	targetServer := startTargetServer()
	defer targetServer.Shutdown(context.TODO())

	file, err := ioutil.TempFile(os.TempDir(), "config.json")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(file.Name())

	file.Write([]byte(`[{
		"url": "/hello/:name",
		"layout": { "path": "/layouts/test_layout", "metadata": { "foo": "test_layout" }},
		"fragments": [
			{ "path": "header", "metadata": { "foo": "header" }},
			{ "path": "body",   "metadata": { "foo": "body" }},
			{ "path": "footer", "metadata": { "foo": "footer" }}
		]
	}]`))

	file.Close()

	viewProxyServer := NewServer("http://localhost:9994")
	viewProxyServer.Port = 9998
	viewProxyServer.Logger = log.New(os.Stdout, "", log.Ldate|log.Ltime)

	err = viewProxyServer.LoadRoutesFromFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	defer viewProxyServer.Close()

	resp, err := http.Get("http://localhost:9998/hello/world")

	if err != nil {
		t.Fatal(err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	expected := "<html><body>hello world</body></html>"

	assert.Equal(t, expected, string(body))
}

func TestPassThroughEnabled(t *testing.T) {
	targetServer := startTargetServer()
	defer targetServer.Shutdown(context.TODO())

	viewProxyServer := NewServer("http://localhost:9994")
	viewProxyServer.Port = 9995
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)
	viewProxyServer.PassThrough = true

	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	defer viewProxyServer.Close()

	resp, err := http.Get("http://localhost:9995/oops")

	if err != nil {
		t.Fatal(err)
	}

	body, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, 500, resp.StatusCode)
	assert.Equal(t, "Something went wrong", string(body))
}

func TestPassThroughDisabled(t *testing.T) {
	targetServer := startTargetServer()
	defer targetServer.Shutdown(context.TODO())

	viewProxyServer := NewServer("http://localhost:9994")
	viewProxyServer.Port = 9993
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)
	viewProxyServer.PassThrough = false

	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	defer viewProxyServer.Close()

	resp, err := http.Get("http://localhost:9993/hello/world")

	if err != nil {
		t.Fatal(err)
	}

	body, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, "404 not found", string(body))
}

func TestPassThroughSetsCorrectHeaders(t *testing.T) {
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)

		assert.Equal(t, "", r.Header.Get("Keep-Alive"), "Expected Keep-Alive to be filtered")
		assert.NotEqual(t, "", r.Header.Get("X-Forwarded-For"))
		assert.Equal(t, "localhost:9993", r.Header.Get("X-Forwarded-Host"))
	}))

	viewProxyServer := NewServer(server.URL)
	viewProxyServer.Port = 9993
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)
	viewProxyServer.PassThrough = true

	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	defer viewProxyServer.Close()

	_, err := http.Get("http://localhost:9993/hello/world")

	assert.Nil(t, err)

	select {
	case <-done:
		server.Close()
	}
}

func TestPassThroughPostRequest(t *testing.T) {
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)

		body, err := io.ReadAll(r.Body)

		assert.Nil(t, err)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "hello", string(body))
	}))

	viewProxyServer := NewServer(server.URL)
	viewProxyServer.Port = 9993
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)
	viewProxyServer.PassThrough = true

	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	defer viewProxyServer.Close()

	_, err := http.Post("http://localhost:9993/hello/world", "text/plain", strings.NewReader("hello"))

	assert.Nil(t, err)

	select {
	case <-done:
		server.Close()
	}
}

func TestFragmentSendsVerifiableHmacWhenSet(t *testing.T) {
	done := make(chan struct{})
	secret := "6ccd9547b7042e0f1101ce68931d6b2c"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)

		time := r.Header.Get("X-Authorization-Time")
		assert.NotEqual(t, "", time, "Expected X-Authorization-Time header to be present")

		key := fmt.Sprintf("%s?%s,%s", r.URL.Path, r.URL.RawQuery, time)

		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(
			[]byte(key),
		)

		authorization := r.Header.Get("Authorization")
		assert.NotEqual(t, "", authorization, "Expected Authorization header to be present")

		expected := hex.EncodeToString(mac.Sum(nil))

		assert.Equal(t, expected, authorization)

		w.WriteHeader(http.StatusOK)
	}))

	viewProxyServer := NewServer(server.URL)
	viewProxyServer.Port = 9993
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)
	viewProxyServer.Get("/hello/:name", NewFragment("/foo"), []*Fragment{})
	viewProxyServer.HmacSecret = secret

	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	defer viewProxyServer.Close()

	_, err := http.Get("http://localhost:9993/hello/world")
	assert.Nil(t, err)

	<-done

	server.Close()
}

func TestFragmentSetsCorrectHeaders(t *testing.T) {
	layoutDone := make(chan bool)
	fragmentDone := make(chan bool)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/foo" {
			defer close(layoutDone)
		} else if r.URL.Path == "/bar" {
			defer close(fragmentDone)
		}
		assert.Equal(t, "", r.Header.Get("Keep-Alive"), "Expected Keep-Alive to be filtered")
		assert.NotEqual(t, "", r.Header.Get("X-Forwarded-For"))
		assert.Equal(t, "localhost:9993", r.Header.Get("X-Forwarded-Host"))
		w.WriteHeader(http.StatusOK)
	}))

	viewProxyServer := NewServer(server.URL)
	viewProxyServer.Port = 9993
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)
	viewProxyServer.Get("/hello/:name", NewFragment("/foo"), []*Fragment{NewFragment("/bar")})

	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	defer viewProxyServer.Close()

	_, err := http.Get("http://localhost:9993/hello/world")

	assert.Nil(t, err)

	<-layoutDone
	<-fragmentDone

	server.Close()
}

func TestSupportsGzip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer

		gzWriter := gzip.NewWriter(&b)

		if r.URL.Path == "/layout" {
			gzWriter.Write([]byte("<body>{{{VIEW_PROXY_CONTENT}}}</body>"))
		} else if r.URL.Path == "/fragment" {
			gzWriter.Write([]byte("wow gzipped!"))
		} else {
			panic("Unexpected URL")
		}

		gzWriter.Close()

		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(b.Bytes())
	}))

	viewProxyServer := NewServer(server.URL)
	viewProxyServer.Port = 9993
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)
	viewProxyServer.Get("/hello/:name", NewFragment("/layout"), []*Fragment{NewFragment("/fragment")})

	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	defer viewProxyServer.Close()

	req, err := http.NewRequest(http.MethodGet, "http://localhost:9993/hello/world", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	assert.Nil(t, err)

	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	assert.Nil(t, err)

	gzReader, err := gzip.NewReader(resp.Body)
	assert.Nil(t, err)

	body, err := ioutil.ReadAll(gzReader)
	assert.Nil(t, err)

	assert.Equal(t, "<body>wow gzipped!</body>", string(body))

	server.Close()
}

func TestPrerequestCallback(t *testing.T) {
	done := make(chan struct{})

	server := NewServer("http://fake.net")
	server.PreRequest = func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		w.Header().Set("x-viewproxy", "true")
		assert.Equal(t, "192.168.1.1", r.RemoteAddr)
	}

	fakeWriter := httptest.NewRecorder()
	fakeRequest := httptest.NewRequest("GET", "/", nil)
	fakeRequest.RemoteAddr = "192.168.1.1"

	server.ServeHTTP(fakeWriter, fakeRequest)

	assert.Equal(t, "true", fakeWriter.Header().Get("x-viewproxy"))

	<-done
}

func startTargetServer() *http.Server {
	instance := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()

		w.Header().Set("EtAg", "1234")
		w.Header().Set("X-Name", "viewproxy")

		if r.URL.Path == "/layouts/test_layout" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html>{{{VIEW_PROXY_CONTENT}}}</html>"))
		} else if r.URL.Path == "/header" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<body>"))
		} else if r.URL.Path == "/body" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fmt.Sprintf("hello %s", params.Get("name"))))
		} else if r.URL.Path == "/footer" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("</body>"))
		} else if r.URL.Path == "/oops" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Something went wrong"))
		} else {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("target: 404 not found"))
		}
	})

	testServer := &http.Server{Addr: ":9994", Handler: instance}
	go func() {
		if err := testServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	return testServer
}
