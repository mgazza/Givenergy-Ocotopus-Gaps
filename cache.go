package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
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

	// Read the request body into memory so we can hash it
	// and also send it on to the next transport.
	var bodyBytes []byte
	if req.Body != nil {
		// ReadAll consumes the entire body
		bodyBytes, _ = ioutil.ReadAll(req.Body)
		// Reassign a new ReadCloser so the next transport can still read it.
		req.Body = ioutil.NopCloser(strings.NewReader(string(bodyBytes)))
	}

	// Build the key for caching. We ignore headers, so only method, URL, and body are used.
	cacheKey := cacheKey(req.Method, req.URL.String(), bodyBytes)

	// Construct the full file path in the cache directory.
	cacheFilePath := c.cacheFilePath(cacheKey)

	// If we have a cached file, try to load it.
	if _, err := os.Stat(cacheFilePath); err == nil {
		// Cached file exists; load and return its contents.
		return c.loadCachedResponse(cacheFilePath, req)
	}

	// Otherwise, do a real round trip.
	resp, err := c.UnderlyingTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body into memory so we can save it.
	respBodyBytes, err := ioutil.ReadAll(resp.Body)
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

// cacheKey builds a SHA-256 hash string from method, url, and request body.
func cacheKey(method, url string, body []byte) string {
	hash := sha256.New()
	hash.Write([]byte(method))
	hash.Write([]byte(url))
	if len(body) > 0 {
		hash.Write(body)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// cacheFilePath returns the path to the cache file for the given key.
func (c *CachingRoundTripper) cacheFilePath(key string) string {
	return fmt.Sprintf("%s/%s.json", c.CacheDir, key)
}

// loadCachedResponse reads the cached file, deserializes it, and returns an *http.Response.
func (c *CachingRoundTripper) loadCachedResponse(path string, req *http.Request) (*http.Response, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cr cachedResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return nil, err
	}

	// Build an http.Response from the cached data
	return buildHTTPResponse(req, cr), nil
}

// saveCachedResponse saves the response struct to a file in JSON format.
func saveCachedResponse(path string, cr *cachedResponse) error {
	data, err := json.MarshalIndent(cr, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0644)
}

// buildHTTPResponse constructs a new *http.Response from cachedResponse data.
func buildHTTPResponse(req *http.Request, cr cachedResponse) *http.Response {
	return &http.Response{
		Status:        cr.Status,
		StatusCode:    cr.StatusCode,
		Proto:         cr.Proto,
		Header:        cr.Header,
		Body:          ioutil.NopCloser(strings.NewReader(string(cr.Body))),
		ContentLength: int64(len(cr.Body)),
		Request:       req,
	}
}
