// services.go
package main

import (
	"fmt"
	"log"
	"net/http"
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

// FetchHalfHourlyInverterData retrieves half-hour usage data from GivEnergy with pagination.
func (s *GivEnergyService) FetchHalfHourlyInverterData(out map[time.Time]*UsageRow, serial string, start, end time.Time) error {
	total := 0
	pageSize := int64(500)
	page := int64(1)

	type GE_UsageRow struct {
		Timestamp                time.Time
		CumulativeImportInverter float64
		CumulativeExportInverter float64
		ImportKWh                float64
		ExportKWh                float64
	}
	data := map[time.Time]*GE_UsageRow{}

	for day := start; day.Before(end); day = day.Add(24 * time.Hour) {
		log.Printf("Getting inverter data for %s\n", day.Format("2006-01-02"))
		params := inverter_data.NewGetDataPoints2Params().
			WithDate(day.Format("2006-01-02")).
			WithInverterSerialNumber(serial).
			WithPageSize(&pageSize).WithPage(&page)

		for {
			response, err := s.Client.InverterData.GetDataPoints2(params, nil)
			if err != nil {
				return fmt.Errorf("failed to fetch inverter data: %w", err)
			}
			log.Printf("Got %d records\n", len(response.Payload.Data))

			for _, d := range response.Payload.Data {
				total++
				hf := time.Time(d.Time).Truncate(30 * time.Minute).Local()
				export := d.Total.Grid.Export
				imported := d.Total.Grid.Import

				row, exists := data[hf]
				if !exists {
					row = &GE_UsageRow{
						Timestamp: hf,
					}
					data[hf] = row
				}

				if export > row.CumulativeExportInverter {
					row.CumulativeExportInverter = export
				}
				if imported > row.CumulativeImportInverter {
					row.CumulativeImportInverter = imported
				}
			}

			if response.Payload.Meta.CurrentPage == response.Payload.Meta.LastPage {
				break
			}
			page++
			log.Printf("...Page %d\n", page)
		}
	}

	var previous *GE_UsageRow
	for t := start; t.Before(end); t = t.Add(30 * time.Minute) {
		row, exists := data[t]
		if !exists {
			if previous != nil {
				c := *previous
				row = &c
				data[t] = row
			}
		} else {
			if previous != nil {
				row.ImportKWh = row.CumulativeImportInverter - previous.CumulativeImportInverter
				row.ExportKWh = row.CumulativeExportInverter - previous.CumulativeExportInverter
			}
			previous = row
		}
	}

	for k, v := range data {
		o, ok := out[k]
		if !ok {
			on := UsageRow{
				Timestamp: v.Timestamp,
			}
			o = &on
			out[k] = o
		}
		o.CumulativeImportInverter = &v.CumulativeImportInverter
		o.CumulativeExportInverter = &v.CumulativeExportInverter
		o.GE_ImportKWh = &v.ImportKWh
		o.GE_ExportKWh = &v.ExportKWh
	}

	log.Printf("Fetched %d GivEnergy records", total)

	return nil
}
