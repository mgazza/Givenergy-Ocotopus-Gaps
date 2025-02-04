// services.go
package main

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	httptransport "github.com/go-openapi/runtime/client"
	strfmt "github.com/go-openapi/strfmt"
	giv "github.com/mgazza/go-givenergy/client"
	"github.com/mgazza/go-givenergy/client/inverter_data"
)

// GivEnergyService handles interactions with the GivEnergy API.
type GivEnergyService struct {
	Client *giv.GivEnergyAPIDocumentationV1350
}

// NewGivEnergyService creates a new GivEnergyService with pre-configured authentication.
func NewGivEnergyService(tr http.RoundTripper, bearerToken string) *GivEnergyService {
	cfg := giv.DefaultTransportConfig()
	transport := httptransport.New(cfg.Host, cfg.BasePath, cfg.Schemes)
	transport.Transport = tr
	transport.DefaultAuthentication = httptransport.BearerToken(bearerToken)

	client := giv.New(transport, strfmt.Default)
	return &GivEnergyService{
		Client: client,
	}
}

// FetchHalfHourlyInverterData retrieves half-hourly usage data using interpolation.
func (s *GivEnergyService) FetchHalfHourlyInverterData(out map[time.Time]*UsageRow, serial string, start, end time.Time) error {
	total := 0
	pageSize := int64(500)
	data := []struct {
		timestamp        time.Time
		cumulativeImport float64
		cumulativeExport float64
	}{}

	// Fetch daily data from GivEnergy with pagination
	for day := start; day.Before(end); day = day.Add(24 * time.Hour) {
		log.Printf("Fetching inverter data for %s", day.Format("2006-01-02"))
		page := int64(1)

		for {
			params := inverter_data.NewGetDataPoints2Params().
				WithDate(day.Format("2006-01-02")).
				WithInverterSerialNumber(serial).
				WithPageSize(&pageSize).
				WithPage(&page)

			response, err := s.Client.InverterData.GetDataPoints2(params, nil)
			if err != nil {
				return fmt.Errorf("failed to fetch inverter data: %w", err)
			}

			for _, d := range response.Payload.Data {
				timestamp := time.Time(d.Time).Local()
				data = append(data, struct {
					timestamp        time.Time
					cumulativeImport float64
					cumulativeExport float64
				}{timestamp, d.Total.Grid.Import, d.Total.Grid.Export})
				total++
			}

			if response.Payload.Meta.CurrentPage >= response.Payload.Meta.LastPage {
				break
			}
			page++
		}
	}

	// Sort data by timestamp
	sort.Slice(data, func(i, j int) bool { return data[i].timestamp.Before(data[j].timestamp) })

	// Interpolate cumulative values at exact half-hour marks
	var lastTime time.Time
	var lastImport, lastExport float64

	for t := start.Truncate(30 * time.Minute); t.Before(end); t = t.Add(30 * time.Minute) {
		var interpImport, interpExport float64
		var found bool
		for i := 1; i < len(data); i++ {
			if data[i].timestamp.After(t) {
				prev := data[i-1]
				next := data[i]
				factor := float64(t.Sub(prev.timestamp)) / float64(next.timestamp.Sub(prev.timestamp))
				interpImport = prev.cumulativeImport + factor*(next.cumulativeImport-prev.cumulativeImport)
				interpExport = prev.cumulativeExport + factor*(next.cumulativeExport-prev.cumulativeExport)
				found = true
				break
			}
		}
		if !found && len(data) > 0 {
			interpImport = data[len(data)-1].cumulativeImport
			interpExport = data[len(data)-1].cumulativeExport
		}

		// Adjust timestamps by shifting back by 30 minutes to fix misalignment
		adjustedTime := t.Add(-30 * time.Minute)

		row, exists := out[adjustedTime]
		if !exists {
			row = &UsageRow{Timestamp: adjustedTime}
			out[adjustedTime] = row
		}
		row.CumulativeImportInverter = &interpImport
		row.CumulativeExportInverter = &interpExport

		if !lastTime.IsZero() {
			importDelta := interpImport - lastImport
			exportDelta := interpExport - lastExport
			row.GE_ImportKWh = &importDelta
			row.GE_ExportKWh = &exportDelta
		}
		lastTime = adjustedTime
		lastImport = interpImport
		lastExport = interpExport
	}

	log.Printf("Processed %d GivEnergy records with interpolated cumulative values and derived usage, with corrected timestamps", total)
	return nil
}
