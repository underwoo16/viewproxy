package viewproxy

import (
	"reflect"
	"strings"
	"testing"

	fragment "github.com/blakewilliams/viewproxy/pkg/fragment"
	"github.com/stretchr/testify/require"
)

func TestRoute_MatchParts(t *testing.T) {
	tests := map[string]struct {
		routePath   string
		providedUrl string
		want        bool
	}{
		"root":                     {routePath: "/", providedUrl: "/", want: true},
		"mismatched root route":    {routePath: "/", providedUrl: "/hello-world", want: false},
		"matching static routes":   {routePath: "/hello/world", providedUrl: "/hello/world", want: true},
		"mismatched static routes": {routePath: "/hello/world", providedUrl: "/hello/false", want: false},
		"valid dynamic route":      {routePath: "/hello/:name", providedUrl: "/hello/world", want: true},
		"invalid dynamic route":    {routePath: "/hello/:name", providedUrl: "/hello/world/wow", want: false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			route := newRoute(test.routePath, map[string]string{}, fragment.Define(""), fragment.Collection{})
			providedUrlParts := strings.Split(test.providedUrl, "/")
			got := route.matchParts(providedUrlParts)

			if got != test.want {
				t.Fatalf("expected route %s to match URL %s", test.routePath, test.providedUrl)
			}
		})
	}
}

func TestRoute_ParametersFor(t *testing.T) {
	tests := map[string]struct {
		routePath   string
		providedUrl string
		want        map[string]string
	}{
		"simple":      {routePath: "/", providedUrl: "/", want: map[string]string{}},
		"multi false": {routePath: "/hello/:name", providedUrl: "/hello/world", want: map[string]string{"name": "world"}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			route := newRoute(test.routePath, map[string]string{}, fragment.Define(""), fragment.Collection{})
			providedUrlParts := strings.Split(test.providedUrl, "/")
			got := route.parametersFor(providedUrlParts)

			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("expected route %v with URL %s to have params: %v\n got: %v", test.routePath, test.providedUrl, test.want, got)
			}
		})
	}
}

func TestRoute_Validate(t *testing.T) {
	testCases := map[string]struct {
		routePath   string
		layout      *fragment.Definition
		fragments   fragment.Collection
		errorString string
		valid       bool
	}{
		"static routes": {
			routePath: "/foo",
			layout:    fragment.Define("/foo/layout"),
			fragments: fragment.Collection{fragment.Define("body")},
			valid:     true,
		},
		"dynamic route matching": {
			routePath: "/hello/:name",
			layout:    fragment.Define("/_viewproxy/hello/:name/layout"),
			fragments: fragment.Collection{fragment.Define("/_viewproxy/hello/:name/body")},
			valid:     true,
		},
		"dynamic route layout not matching": {
			routePath:   "/hello/:name",
			layout:      fragment.Define("/_viewproxy/hello/:login/layout"),
			fragments:   fragment.Collection{fragment.Define("/_viewproxy/hello/:name/body")},
			valid:       false,
			errorString: "dynamic route /hello/:name has mismatched fragment route /_viewproxy/hello/:login/layout",
		},
		"dynamic route body not matching": {
			routePath:   "/hello/:name",
			layout:      fragment.Define("/_viewproxy/hello/:name/layout"),
			fragments:   fragment.Collection{fragment.Define("/_viewproxy/hello/:login/body")},
			valid:       false,
			errorString: "dynamic route /hello/:name has mismatched fragment route /_viewproxy/hello/:login/body",
		},
		"static route with dynamic layout": {
			routePath:   "/foo",
			layout:      fragment.Define("/_viewproxy/hello/:name/layout"),
			fragments:   fragment.Collection{fragment.Define("body")},
			valid:       false,
			errorString: "static route /foo has mismatched fragment route /_viewproxy/hello/:name/layout",
		},
		"static route with dynamic body": {
			routePath:   "/foo",
			layout:      fragment.Define("/_viewproxy/foo/layout"),
			fragments:   fragment.Collection{fragment.Define("/_viewproxy/hello/:name/body")},
			valid:       false,
			errorString: "static route /foo has mismatched fragment route /_viewproxy/hello/:name/body",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			route := newRoute(tc.routePath, map[string]string{}, tc.layout, tc.fragments)

			valid, err := route.Validate()

			if tc.valid {
				require.True(t, valid)
				require.NoError(t, err)
			} else {
				require.False(t, valid)
				require.EqualError(t, err, tc.errorString)
			}
		})
	}
}

func TestFragment_HasDynamicParts(t *testing.T) {
	testCases := map[string]struct {
		input string
		want  bool
	}{
		"no dynamic parts": {input: "/foo/bar", want: false},
		"dynamic parts":    {input: "/:hello/namme", want: true},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			definition := newRoute(tc.input, map[string]string{}, fragment.Define("my_layout"), fragment.Collection{})
			require.Equal(t, tc.want, definition.HasDynamicParts())
		})
	}
}

func TestLayout(t *testing.T) {
	route := newRoute("/", map[string]string{}, fragment.Define("my_layout"), fragment.Collection{})

	require.Equal(t, route.LayoutFragment.Path, "my_layout")
}
