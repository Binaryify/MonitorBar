package mitv

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestIdentityMismatchBlocksRequests(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, `{"msg":"success","data":{}}`) }))
	defer server.Close()
	l := New("unused")
	l.client = &http.Client{Transport: rewriteTransport{server.URL}}
	l.checkIdentity = func() error { return errors.New("identity changed") }
	if err := l.Press("volumeup"); err == nil {
		t.Fatal("control should be refused")
	}
	if calls != 0 {
		t.Fatal("sent a command to mismatched device")
	}
}

type rewriteTransport struct{ url string }

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, _ := url.Parse(r.url)
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	return http.DefaultTransport.RoundTrip(req)
}
