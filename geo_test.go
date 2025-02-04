package main

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPopulateGeoData(t *testing.T) {
	mockRoundTripper := &MockRoundTripper{
		Handler: func(req *http.Request) (*http.Response, error) {
			responseBody := ""

			if strings.Contains(req.URL.Path, "/usersservice/v2/login") {
				responseBody = `{
				  "username": "wibble",
				  "email": "example@example.com",
				  "displayName": "example@example.com",
				  "validated": true,
				  "accessToken": "wibble"
				}`
			} else if strings.Contains(req.URL.Path, "/api/userapi/v2/user/detail-systems") {
				responseBody = `{
				  "systemRoles": [
					{
					  "name": "Home",
					  "systemId": "123",
					  "roles": ["READ", "WRITE"]
					}
				  ],
				  "systemDetails": [
					{
					  "name": "Home",
					  "devices": [
						{
						  "deviceType": "TRIO_II_TB_GEO",
						  "sensorType": 94,
						  "nodeId": 0
						}
					  ],
					  "systemId": "123"
					}
				  ]
				}`
			} else if strings.Contains(req.URL.Path, "/epochservice/v1/system/") {
				// Simulating API response
				responseBody = `[
				  {
					"systemUUID": "1e2d7705-1d18-4e31-9a83-0d62b3733123",
					"startTimestamp": 1733709600.000000000,
					"readings": [
					  {
						"energyType": "IMPORT",
						"tierType": "VARIABLE",
						"duration": 900,
						"energyWattHours": 1470,
						"milliPenceCost": 34559
					  },
					  {
						"energyType": "GAS_ENERGY",
						"tierType": "VARIABLE",
						"duration": 900,
						"energyWattHours": 500,
						"milliPenceCost": 12000
					  }
					]
				  },
				  {
					"systemUUID": "1e2d7705-1d18-4e31-9a83-0d62b3733123",
					"startTimestamp": 1733710500.000000000,
					"readings": [
					  {
						"energyType": "IMPORT",
						"tierType": "VARIABLE",
						"duration": 900,
						"energyWattHours": 1541,
						"milliPenceCost": 36228
					  },
					  {
						"energyType": "GAS_ENERGY",
						"tierType": "VARIABLE",
						"duration": 900,
						"energyWattHours": 600,
						"milliPenceCost": 15000
					  }
					]
				  },
				  {
					"systemUUID": "1e2d7705-1d18-4e31-9a83-0d62b3733123",
					"startTimestamp": 1733711400.000000000,
					"readings": [
					  {
						"energyType": "IMPORT",
						"tierType": "VARIABLE",
						"duration": 900,
						"energyWattHours": 1358,
						"milliPenceCost": 31926
					  },
					  {
						"energyType": "GAS_ENERGY",
						"tierType": "VARIABLE",
						"duration": 900,
						"energyWattHours": 700,
						"milliPenceCost": 18000
					  }
					]
				  }
				]`
			} else {
				t.Fatalf("unhandled request %s", req.URL)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(responseBody))),
				Header:     make(http.Header),
			}, nil
		},
	}

	// Mock service with the fake HTTP response
	mockGeoService, err := NewGeoTogetherService(mockRoundTripper, "user", "password")
	require.NoError(t, err)

	usage := make(map[time.Time]*UsageRow)
	startDate := time.Date(2024, 12, 9, 2, 0, 0, 0, time.Local)
	endDate := startDate.Add(1 * time.Hour) // Testing one-hour window

	// Run function
	err = mockGeoService.PopulateGeoData(usage, startDate, endDate)
	require.NoError(t, err)

	// Expected Aggregated Readings
	expectedReadings := map[time.Time]struct {
		importWh   int64
		gasWh      int64
		importCost int64
		gasCost    int64
	}{
		startDate:                       {1470 + 1541, 500 + 600, 34559 + 36228, 12000 + 15000}, // 02:00 - 02:30
		startDate.Add(30 * time.Minute): {1358, 700, 31926, 18000},                              // 02:30 - 03:00
	}

	// Validate results
	for timestamp, expected := range expectedReadings {
		row, exists := usage[timestamp]
		require.True(t, exists, "Expected data for %s", timestamp)
		require.NotNil(t, row.GEO_ImportWh)
		require.NotNil(t, row.GEO_ImportGasWh)
		require.NotNil(t, row.GEO_ImportMilliPenceCost)
		require.NotNil(t, row.GEO_ImportGasMilliPenceCost)

		require.Equal(t, expected.importWh, *row.GEO_ImportWh, "Mismatch in importWh at %s", timestamp)
		require.Equal(t, expected.gasWh, *row.GEO_ImportGasWh, "Mismatch in gasWh at %s", timestamp)
		require.Equal(t, expected.importCost, *row.GEO_ImportMilliPenceCost, "Mismatch in importCost at %s", timestamp)
		require.Equal(t, expected.gasCost, *row.GEO_ImportGasMilliPenceCost, "Mismatch in gasCost at %s", timestamp)
	}
}
