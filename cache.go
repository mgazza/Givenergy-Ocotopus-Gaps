package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// cachedResponse is a helper struct to store the response fields
// we care about in a simple JSON format.
type cachedResponse struct {
	Status     string              `json:"status"`
	StatusCode int                 `json:"status_code"`
	Proto      string              `json:"proto"`
	Header     map[string][]string `json:"header"`
	Body       []byte              `json:"body"`
}

// CachingRoundTripper implements http.RoundTripper.
type CachingRoundTripper struct {
	// UnderlyingTransport will be used when there's a cache miss.
	// If nil, http.DefaultTransport will be used.
	UnderlyingTransport http.RoundTripper

	// CacheDir is the directory where response files are stored.
	CacheDir string
}

func (c *CachingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if c.UnderlyingTransport == nil {
		c.UnderlyingTransport = http.DefaultTransport
	}

	// Read the request body into memory so we can reuse it for the real request.
	// (We do this even though we aren't hashing it anymore.)
	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	}

	// Build a filename from the method + URL. Then sanitize it.
	fileName := sanitizeFileName(req.Method + "_" + req.URL.String())
	cacheFilePath := filepath.Join(c.CacheDir, fileName+".json")

	// If we have a cached file, try to load it and return it.
	if _, err := os.Stat(cacheFilePath); err == nil {
		return c.loadCachedResponse(cacheFilePath, req)
	}

	// Otherwise, do a real round trip.
	resp, err := c.UnderlyingTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body into memory so we can save it.
	respBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Save response to disk.
	cr := cachedResponse{
		Status:     resp.Status,
		StatusCode: resp.StatusCode,
		Proto:      resp.Proto,
		Header:     resp.Header.Clone(),
		Body:       respBodyBytes,
	}
	if err := saveCachedResponse(cacheFilePath, &cr); err != nil {
		return nil, err
	}

	// We need to return a new http.Response that has a readable Body.
	return buildHTTPResponse(req, cr), nil
}

// loadCachedResponse reads the cached file, deserializes it, and returns an *http.Response.
func (c *CachingRoundTripper) loadCachedResponse(path string, req *http.Request) (*http.Response, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cr cachedResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return nil, err
	}

	return buildHTTPResponse(req, cr), nil
}

// saveCachedResponse saves the response struct to a file in JSON format.
func saveCachedResponse(path string, cr *cachedResponse) error {
	data, err := json.MarshalIndent(cr, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// buildHTTPResponse constructs a new *http.Response from cachedResponse data.
func buildHTTPResponse(req *http.Request, cr cachedResponse) *http.Response {
	return &http.Response{
		Status:        cr.Status,
		StatusCode:    cr.StatusCode,
		Proto:         cr.Proto,
		Header:        cr.Header,
		Body:          io.NopCloser(strings.NewReader(string(cr.Body))),
		ContentLength: int64(len(cr.Body)),
		Request:       req,
	}
}

// sanitizeFileName replaces or removes characters that are invalid or awkward
// for filenames across different operating systems. Adjust as needed.
func sanitizeFileName(name string) string {
	// Replace potentially problematic characters with underscores.
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		"&", "_",
		"=", "_",
		"%", "_",
		" ", "_",
	)
	safe := replacer.Replace(name)

	// Optionally, limit length to prevent extremely long file names:
	// if len(safe) > 200 {
	// 	   safe = safe[:200]
	// }
	return safe
}
