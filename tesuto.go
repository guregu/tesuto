package tesuto

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// HTTP is a test suite wrapper around httptest.Server.
type HTTP struct {
	*httptest.Server
}

// New creates a new test suite.
func New(server *httptest.Server) HTTP {
	return HTTP{
		Server: server,
	}
}

// Test returns a test function suitable for running with t.Run.
func (h HTTP) Test(method string, path string, opts ...TestOption) func(*testing.T) {
	tc := &testCase{
		server:        h.Server,
		method:        method,
		path:          path,
		expectHeaders: make(map[string]string),
	}
	for _, opt := range opts {
		opt(tc)
	}
	return tc.fn()
}

type testCase struct {
	server        *httptest.Server
	method        string
	path          string
	mutateReq     []func(*http.Request)
	input         io.Reader
	jar           *cookiejar.Jar
	expectCode    int
	expectRaw     []byte
	expectJSON    interface{}
	outputCmpOpt  []cmp.Option
	expectHeaders map[string]string
	grabOutput    interface{}
	fatalFailure  bool
}

func (tc *testCase) fn() func(*testing.T) {
	return func(t *testing.T) {
		client := tc.server.Client()
		if tc.jar != nil {
			client.Jar = tc.jar
		}

		req, err := http.NewRequest(tc.method, tc.server.URL+tc.path, tc.input)
		if err != nil {
			t.Fatal(err)
		}
		for _, mut := range tc.mutateReq {
			mut(req)
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		gotRaw, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Error("error reading body:", err)
		}
		t.Log("output:\n", string(gotRaw))

		fail := t.Errorf
		if tc.fatalFailure {
			fail = t.Fatalf
		}

		if tc.expectCode != 0 && resp.StatusCode != tc.expectCode {
			fail("[%s %s] unexpected response code: want %v, got %v", tc.method, tc.path, tc.expectCode, resp.StatusCode)
		}

		for k, v := range tc.expectHeaders {
			if got := resp.Header.Get(k); got != v {
				fail("[%s %s] unexpected response header (%s): want %v, got %v", tc.method, tc.path, k, v, got)
				t.Logf("header dump: %#v", resp.Header)
			}
		}

		if tc.expectRaw != nil {
			if !bytes.Equal(tc.expectRaw, gotRaw) {
				fail("[%s %s] raw output mismatch:\nwant: %s\ngot: %s", tc.method, tc.path, string(tc.expectRaw), string(gotRaw))
			}
		}

		if tc.expectJSON != nil {
			outptr := reflect.New(reflect.TypeOf(tc.expectJSON))
			if err := json.Unmarshal(gotRaw, outptr.Interface()); err != nil {
				t.Fatal(err)
			}
			output := outptr.Elem().Interface()
			if diff := cmp.Diff(tc.expectJSON, output, tc.outputCmpOpt...); diff != "" {
				fail("[%s %s] output mismatch (-want +got):\n%s", tc.method, tc.path, diff)
			}
		}

		if tc.grabOutput != nil {
			if err := json.Unmarshal(gotRaw, tc.grabOutput); err != nil {
				t.Fatal(err)
			}
		}
	}
}

type TestOption func(*testCase)

// WithInput specifies the request body data for this test.
func WithInput(r io.Reader) TestOption {
	return func(tc *testCase) {
		tc.input = r
	}
}

// WithInput specifies the JSON request body data for this test and expects application/json Content-Type.
// The header expectation can be overriden with WithHeader.
func WithJSONInput(input interface{}) TestOption {
	return func(tc *testCase) {
		raw, err := json.Marshal(tc.input)
		if err != nil {
			panic(err)
		}
		tc.input = bytes.NewReader(raw)

		tc.mutateReq = append(tc.mutateReq, func(r *http.Request) {
			r.Header.Set("Content-Type", "application/json")
		})
	}
}

// WithInput specifies the form request body data for this test and expects application/x-www-form-urlencoded Content-Type.
// The header expectation can be overriden with WithHeader.
func WithFormInput(values url.Values) TestOption {
	return func(tc *testCase) {
		raw := values.Encode()
		tc.input = strings.NewReader(raw)

		tc.mutateReq = append(tc.mutateReq, func(r *http.Request) {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		})
	}
}

// WithHeader specifies a header to be added to the request for this test.
func WithHeader(name, value string) TestOption {
	return func(tc *testCase) {
		tc.mutateReq = append(tc.mutateReq, func(r *http.Request) {
			// special case Content-Type to allow people to override WithXInput's automatic settings
			if http.CanonicalHeaderKey(name) == "Content-Type" {
				r.Header.Set(name, value)
				return
			}
			r.Header.Add(name, value)
		})
	}
}

func WithCookieJar(jar *cookiejar.Jar) TestOption {
	return func(tc *testCase) {
		tc.jar = jar
	}
}

// ExpectStatusCode specifies the expected HTTP status code of the response.
func ExpectStatusCode(code int) TestOption {
	return func(tc *testCase) {
		tc.expectCode = code
	}
}

// ExpectStatusCode specifies an expected HTTP header of the response.
func ExpectHeader(name, value string) TestOption {
	return func(tc *testCase) {
		tc.expectHeaders[name] = value
	}
}

// ExpectRawResponse specifies the exact body expected of the response.
func ExpectRawResponse(body []byte) TestOption {
	return func(tc *testCase) {
		tc.expectRaw = body
	}
}

// ExpectJSONResponse specifies a JSON object that should match the response.
// The response will be decoded into the same type as the specified output and compared.
// Comparison options can be specified.
func ExpectJSONResponse(output interface{}, compareOpt ...cmp.Option) TestOption {
	return func(tc *testCase) {
		tc.expectJSON = output
		tc.outputCmpOpt = compareOpt
	}
}

// GrabJSONResponse takes a pointer to an object and unmarshals the response into it.
// Use this for examining data outside of the test.
func GrabJSONResponse(out interface{}) TestOption {
	return func(tc *testCase) {
		tc.grabOutput = out
	}
}

// FatalFailure will make this test fail-fast.
func FatalFailure() TestOption {
	return func(tc *testCase) {
		tc.fatalFailure = true
	}
}

// NotEmpty is a comparison option that requires both things to not be empty. That is, different from their zero values.
func NotEmpty(name string) cmp.Option {
	return cmp.FilterPath(func(p cmp.Path) bool {
		return p.String() == name
	}, cmp.FilterValues(func(x, y interface{}) bool {
		return !reflect.DeepEqual(x, reflect.Zero(reflect.TypeOf(x)).Interface()) &&
			!reflect.DeepEqual(y, reflect.Zero(reflect.TypeOf(y)).Interface())
	}, cmp.Comparer(func(_, _ interface{}) bool { return true })))
}

// IgnoreField is a comparison option that ignores the given field, like "Foo" or "Foo.Bar".
func IgnoreField(name string) cmp.Option {
	return cmp.FilterPath(func(p cmp.Path) bool {
		return p.String() == name
	}, cmp.Ignore())
}

func IgnoreUnexported(types ...interface{}) cmp.Option {
	return cmpopts.IgnoreUnexported(types...)
}

func EquateApproxTime(margin time.Duration) cmp.Option {
	return cmpopts.EquateApproxTime(margin)
}

func SortSlices(lessFunc interface{}) cmp.Option {
	return cmpopts.SortSlices(lessFunc)
}

func ParseHTML(t *testing.T, body string) *goquery.Document {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		t.Fatalf("couldn't parse HTML (error: %v)\n body:\n\t%s", err, body)
	}
	return doc
}

func ParseURL(t *testing.T, href string) *url.URL {
	url, err := url.Parse(href)
	if err != nil {
		t.Fatalf("couldn't parse URL (error: %v): %s", err, href)
	}
	return url
}
