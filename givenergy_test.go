package main

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFetchHalfHourlyInverterData(t *testing.T) {
	// Expected call to the GivEnergy API: GET /inverter/{serial}/data-points/{date}
	mockRoundTripper := &MockRoundTripper{
		Handler: func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/v1/inverter/ABC12345/data-points/2025-01-01", req.URL.Path, "Unexpected request URL")

			// Canned response
			responseBody := `{
				"data": [
					{"time": "2025-01-01T00:00:00Z", "total": {"grid": {"import": 1842.3, "export": 1629.9}}},
					{"time": "2025-01-01T00:30:00Z", "total": {"grid": {"import": 1845.4, "export": 1630}}}
				],
				"meta": {"current_page": 1, "last_page": 1}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(responseBody))),
				Header:     make(http.Header),
			}, nil
		},
	}

	givService := NewGivEnergyService(mockRoundTripper, "dummyBearerToken")
	serial := "ABC12345"
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(2025, 1, 1, 23, 59, 59, 0, time.Local)

	data := map[time.Time]*UsageRow{}
	err := givService.FetchHalfHourlyInverterData(data, serial, start, end)
	require.NoError(t, err, "Expected no error while fetching inverter data")
	require.Len(t, data, 48, "Expected 48 data points")
	require.Equal(t, 1845.4, *data[start].CumulativeImportInverter, "Unexpected first cumulative import")
}
