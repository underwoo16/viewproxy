package multiplexer

import "net/http"

type Tripper interface {
	Request(r *http.Request) (*http.Response, error)
}

type StandardTripper struct {
	client *http.Client
}

// Creates a new instance of a Tripper. The passed in client is modified to
// have no cookie jar and to not follow redirects.
func NewStandardTripper(client *http.Client) *StandardTripper {
	client.Jar = nil
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &StandardTripper{client: client}
}

func (t *StandardTripper) Request(r *http.Request) (*http.Response, error) {
	return t.client.Do(r)
}
