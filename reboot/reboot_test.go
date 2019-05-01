package reboot

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/m-lab/go/prometheusx/promtest"
	"github.com/m-lab/rebot/node"
)

type RoundTripFunc func(req *http.Request) *http.Response

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

// NewTestClient returns a *http.Client with Transport replaced to avoid making
// real calls.
func NewTestClient(fn RoundTripFunc) *http.Client {
	return &http.Client{
		Transport: RoundTripFunc(fn),
	}
}
func Test_rebootMany(t *testing.T) {

	// Set up the RoundTripFunc to return values useful for testing.
	client := NewTestClient(func(req *http.Request) *http.Response {
		fmt.Println(req.Header.Get("Authorization"))
		// Check that the Authorization header contains the base64-encoded
		// credentials "user:pass".
		// $ echo -n user:pass | base64
		if req.Header.Get("Authorization") != "Basic dXNlcjpwYXNz" {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       ioutil.NopCloser(bytes.NewBufferString("unauthorized")),
				Header:     make(http.Header),
			}
		}

		// URL must be correct
		if strings.HasPrefix(req.URL.String(), "http://localhost:8080/v1/reboot?host=") {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       ioutil.NopCloser(bytes.NewBufferString("not found")),
				Header:     make(http.Header),
			}
		}

		// "host" parameter must not be empty.
		host := req.URL.Query().Get("host")
		if host == "" {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body: ioutil.NopCloser(bytes.NewBufferString(
					"URL parameter 'host' is missing")),
				Header: make(http.Header),
			}
		}

		// mlab4 nodes are always failing.
		if strings.HasPrefix(host, "mlab4") {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       ioutil.NopCloser(bytes.NewBufferString("i/o error")),
				Header:     make(http.Header),
			}
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Server power operation successful.")),
			Header:     make(http.Header),
		}
	})

	rebooter := NewHTTPRebooter(client, "/v1/reboot", "user", "pass")

	// These must succeed.
	toReboot := []node.Node{
		{
			Name: "mlab1.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
		{
			Name: "mlab2.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
	}
	want := map[string]error{}

	t.Run("success-all-nodes-rebooted", func(t *testing.T) {
		if got := rebooter.Many(toReboot); !reflect.DeepEqual(got, want) {
			t.Errorf("rebootMany() = %v, want %v", got, want)
		}
	})

	// mlab4.* nodes always fail.
	toReboot = []node.Node{
		{
			Name: "mlab4.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
	}

	t.Run("failure-exit-code-non-zero", func(t *testing.T) {
		got := rebooter.Many(toReboot)
		if err, ok := got["mlab4.lga0t.measurement-lab.org"]; !ok || err == nil {
			t.Errorf("rebootMany() = %v, key not in map or err == nil", got)
		}
	})

	t.Run("success-empty-slice", func(t *testing.T) {
		got := rebooter.Many([]node.Node{})
		if got == nil || len(got) != 0 {
			t.Errorf("rebootMany() = %v, error map not empty.", got)
		}
	})

	toReboot = []node.Node{
		{
			Name: "mlab1.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
		{
			Name: "mlab2.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
		{
			Name: "mlab3.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
		{
			Name: "mlab1.lga1t.measurement-lab.org",
			Site: "lga1t",
		},
		{
			Name: "mlab2.lga1t.measurement-lab.org",
			Site: "lga1t",
		},
		{
			Name: "mlab3.lga1t.measurement-lab.org",
			Site: "lga1t",
		},
	}
	t.Run("success-too-many-nodes", func(t *testing.T) {
		got := rebooter.Many(toReboot)
		if got == nil || len(got) != 0 {
			t.Errorf("rebootMany() = %v, error map not empty.", got)
		}
	})

	toReboot = []node.Node{
		{
			Name: "mlab1.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
	}

	t.Run("failure-cannot-create-request", func(t *testing.T) {
		// Swap newHTTPRequest to simulate failure during request creation.
		oldHTTPRequestFunc := newHTTPRequest
		newHTTPRequest = func(method, url string, body io.Reader) (*http.Request, error) {
			return nil, errors.New("Error while creating HTTP request")
		}

		got := rebooter.Many(toReboot)
		newHTTPRequest = oldHTTPRequestFunc

		if _, ok := got["mlab1.lga0t.measurement-lab.org"]; !ok {
			t.Errorf("rebootMany() = %v, key not in map", got)
		}
	})

	t.Run("failure-cannot-read-body", func(t *testing.T) {
		// Swap readAll function to simulate failure while reading the
		// response body.
		oldReadAllFunc := readAll
		readAll = func(reader io.Reader) ([]byte, error) {
			return nil, errors.New("Cannot read")
		}

		got := rebooter.Many(toReboot)
		readAll = oldReadAllFunc

		if _, ok := got["mlab1.lga0t.measurement-lab.org"]; !ok {
			t.Errorf("rebootMany() = %v, key not in map", got)
		}

	})

	t.Run("failure-cannot-send-request", func(t *testing.T) {
		// Swap clientDo function to simulate failure while sending
		// the request.
		oldClientDo := clientDo
		clientDo = func(r *HTTPRebooter, req *http.Request) (*http.Response, error) {
			return nil, errors.New("Cannot send request")
		}

		got := rebooter.Many(toReboot)
		clientDo = oldClientDo

		if _, ok := got["mlab1.lga0t.measurement-lab.org"]; !ok {
			t.Errorf("rebootMany() = %v, key not in map", got)
		}

	})

}

func TestMetrics(t *testing.T) {
	metricRebootRequests.WithLabelValues("x", "x", "x", "x")
	promtest.LintMetrics(t)
}
