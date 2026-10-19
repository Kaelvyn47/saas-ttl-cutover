package cutover

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAddDomainDecodesBusinessRejectionBeforeStatus(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing bearer auth")
		}
		return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":false,"data":null,"error":{"code":"INVALID_INPUT","message":"domain is required"},"metadata":null}`))}, nil
	})}
	c := &Client{BaseURL: "https://example.invalid", APIKey: "test-key", HTTP: httpClient}
	_, err := c.AddDomain(context.Background(), "", "acct-42", "req-1")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "INVALID_INPUT" || apiErr.HTTPStatus != 400 {
		t.Fatalf("error = %#v", err)
	}
}
