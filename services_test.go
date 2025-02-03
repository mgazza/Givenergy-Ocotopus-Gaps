// services_test.go
package main

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// MockRoundTripper is a mock implementation of http.RoundTripper.
type MockRoundTripper struct {
	Handler func(req *http.Request) (*http.Response, error)
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.Handler(req)
}

func TestFetchTariffs(t *testing.T) {
	// Expected call to the Octopus API: GET /products/{productCode}/electricity-tariffs/{tariffCode}/standard-unit-rates/
	mockRoundTripper := &MockRoundTripper{
		Handler: func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/v1/products/AGILE-24-10-01/electricity-tariffs/E-1R-AGILE-24-10-01-M/standard-unit-rates/", req.URL.Path, "Unexpected request URL")

			// Canned response
			responseBody := `{
				"count": 288,
				"next": null,
				"previous": null,
				"results": [{"value_inc_vat": 20.769, "valid_from": "2025-01-15T23:30:00Z", "valid_to": "2025-01-16T00:00:00Z"}]
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(responseBody))),
				Header:     make(http.Header),
			}, nil
		},
	}

	octopusService := NewOctopusService(mockRoundTripper, "dummyApiKey")
	start := time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 15, 23, 59, 59, 0, time.UTC)

	tariffs, err := octopusService.FetchTariffs("AGILE-24-10-01", "E-1R-AGILE-24-10-01-M", start, end)
	require.NoError(t, err, "Expected no error while fetching tariffs")
	require.Len(t, tariffs, 1, "Expected 1 tariff")
	require.Equal(t, 20.769, tariffs[0].Rate, "Unexpected tariff rate")
}

func TestFetchHalfHourlyInverterData(t *testing.T) {
	// Expected call to the GivEnergy API: GET /inverter/{serial}/data-points/{date}
	mockRoundTripper := &MockRoundTripper{
		Handler: func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/v1/inverter/ABC12345/data-points/2025-01-01", req.URL.Path, "Unexpected request URL")

			// Canned response
			responseBody := `{
				"data": [
					{"time": "2025-01-01T00:01:05Z", "total": {"grid": {"import": 1842.3, "export": 1629.9}}},
					{"time": "2025-01-01T00:31:27Z", "total": {"grid": {"import": 1845.4, "export": 1630}}}
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
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 23, 59, 59, 0, time.UTC)

	data := map[time.Time]*UsageRow{}
	err := givService.FetchHalfHourlyInverterData(data, serial, start, end)
	require.NoError(t, err, "Expected no error while fetching inverter data")
	require.Len(t, data, 48, "Expected 48 data points")
	require.Equal(t, 1842.3, data[start].CumulativeImportInverter, "Unexpected first cumulative import")
}

func TestGetLastReading(t *testing.T) {
	// Expected call to the Octopus API: GET /electricity-meter-points/{mpan}/meters/{serial}/consumption/
	mockRoundTripper := &MockRoundTripper{
		Handler: func(req *http.Request) (*http.Response, error) {
			// Canned response
			responseBody := `{
				"count": 2,
				"results": [
					{"interval_start": "2025-01-01T00:30:00Z", "interval_end": "2025-01-01T01:00:00Z", "consumption": 0.45},
					{"interval_start": "2025-01-01T00:00:00Z", "interval_end": "2025-01-01T00:30:00Z", "consumption": 123.45}
				]
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(responseBody))),
				Header:     make(http.Header),
			}, nil
		},
	}

	octopusService := NewOctopusService(mockRoundTripper, "dummyApiKey")

	meter := &MeterInfo{
		SerialNumber: "123456789",
		Mpan:         "987654321",
	}

	lastReading, value, err := octopusService.GetLastReading(meter)
	require.NoError(t, err, "Expected no error while fetching last reading")
	require.Equal(t, "2025-01-01T00:30:00Z", lastReading.Format(time.RFC3339), "Unexpected last reading timestamp")
	require.Equal(t, 0.45, value, "Unexpected last reading value")
}

func TestGetMetersAndTariff(t *testing.T) {
	// Expected call to the Octopus API: GET /accounts/{accountId} and GET /products
	mockRoundTripper := &MockRoundTripper{
		Handler: func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/v1/accounts/dummyAccountId" {
				// Response for account details
				responseBody := `{
					"properties": [
						{
							"electricity_meter_points": [
								{
									"mpan": "123456789",
									"meters": [
										{"serial_number": "SN123"}
									],
									"agreements": [
										{"tariff_code": "E-1R-AGILE-24-10-01-M"}
									]
								},
								{
									"mpan": "987654321",
									"meters": [
										{"serial_number": "SN987"}
									],
									"agreements": [
										{"tariff_code": "E-1R-EXPORT-24-10-01-M"}
									],
									"is_export": true
								}
							]
						}
					]
				}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(responseBody))),
					Header:     make(http.Header),
				}, nil
			} else if req.URL.Path == "/v1/products/" {
				// Response for product list
				responseBody := `{
					"results": [
						{"code": "AGILE-24-10-01"},
						{"code": "EXPORT-24-10-01"}
					]
				}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(responseBody))),
					Header:     make(http.Header),
				}, nil
			}

			return nil, nil
		},
	}

	octopusService := NewOctopusService(mockRoundTripper, "dummyApiKey")

	importMeter, exportMeter, _, err := octopusService.GetMetersAndTariff("dummyAccountId")
	require.NoError(t, err, "Expected no error while fetching meters and tariffs")

	require.Equal(t, "123456789", importMeter.Mpan, "Unexpected import meter MPAN")
	require.Equal(t, "E-1R-AGILE-24-10-01-M", importMeter.TariffCode, "Unexpected import tariff code")

	require.Equal(t, "987654321", exportMeter.Mpan, "Unexpected export meter MPAN")
	require.Equal(t, "E-1R-EXPORT-24-10-01-M", exportMeter.TariffCode, "Unexpected export tariff code")
}
