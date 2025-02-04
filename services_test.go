// services_test.go
package main

import (
	"net/http"
)

// MockRoundTripper is a mock implementation of http.RoundTripper.
type MockRoundTripper struct {
	Handler func(req *http.Request) (*http.Response, error)
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.Handler(req)
}
