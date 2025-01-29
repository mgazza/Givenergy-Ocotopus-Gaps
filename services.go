// services.go
package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	httptransport "github.com/go-openapi/runtime/client"
	strfmt "github.com/go-openapi/strfmt"
	giv "github.com/mgazza/go-givenergy/client"
	"github.com/mgazza/go-givenergy/client/inverter_data"
	octopus "github.com/mgazza/go-octopus-energy/client"
	"github.com/mgazza/go-octopus-energy/client/accounts"
	"github.com/mgazza/go-octopus-energy/client/electricity_meter_points"
	"github.com/mgazza/go-octopus-energy/client/products"
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
func (s *GivEnergyService) FetchHalfHourlyInverterData(serial string, start, end time.Time) (map[time.Time]*UsageRow, error) {
	out := make(map[time.Time]*UsageRow)
	pageSize := int64(500)
	page := int64(1)

	for day := start; day.Before(end); day = day.Add(24 * time.Hour) {
		log.Printf("Getting inverter data for %s\n", day.Format("2006-01-02"))
		params := inverter_data.NewGetDataPoints2Params().
			WithDate(day.Format("2006-01-02")).
			WithInverterSerialNumber(serial).
			WithPageSize(&pageSize).WithPage(&page)

		for {
			response, err := s.Client.InverterData.GetDataPoints2(params, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch inverter data: %w", err)
			}
			log.Printf("Got %d records\n", len(response.Payload.Data))

			for _, d := range response.Payload.Data {
				hf := time.Time(d.Time).Truncate(30 * time.Minute)
				export := d.Total.Grid.Export
				imported := d.Total.Grid.Import

				row, exists := out[hf]
				if !exists {
					row = &UsageRow{
						Timestamp: hf,
					}
					out[hf] = row
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

	var previous *UsageRow
	for t := start; t.Before(end); t = t.Add(30 * time.Minute) {
		row, exists := out[t]
		if !exists {
			if previous != nil {
				c := *previous
				row = &c
				out[t] = row
			}
		} else {
			if previous != nil {
				row.ImportKWh = row.CumulativeImportInverter - previous.CumulativeImportInverter
				row.ExportKWh = row.CumulativeExportInverter - previous.CumulativeExportInverter
			}
			previous = row
		}
	}

	return out, nil
}

// OctopusService handles interactions with the Octopus Energy API.
type OctopusService struct {
	Client *octopus.OctopusEnergyRESTAPI
}

// NewOctopusService creates a new OctopusService with pre-configured authentication.
func NewOctopusService(rt http.RoundTripper, apiKey string) *OctopusService {
	cfg := octopus.DefaultTransportConfig()
	transport := httptransport.New(cfg.Host, cfg.BasePath, cfg.Schemes)
	transport.Transport = rt
	transport.DefaultAuthentication = httptransport.BasicAuth(apiKey, "")

	client := octopus.New(transport, strfmt.Default)
	return &OctopusService{
		Client: client,
	}
}

// GetMetersAndTariff fetches meter information and tariff details.
func (s *OctopusService) GetMetersAndTariff(accountID string) (*MeterInfo, *MeterInfo, error) {
	params := accounts.NewGetAccountParams().WithAccountID(accountID)
	response, err := s.Client.Accounts.GetAccount(params, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch account details: %w", err)
	}

	if len(response.Payload.Properties) < 1 {
		return nil, nil, fmt.Errorf("no properties found on the account")
	}

	property := response.Payload.Properties[0]

	// Fetch all products to map product codes
	productParams := products.NewListProductsParams()
	productResponse, err := s.Client.Products.ListProducts(productParams, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch products: %w", err)
	}

	findProductCode := func(productCode string) string {
		for _, p := range productResponse.Payload.Results {
			if strings.Contains(productCode, *p.Code) {
				return *p.Code
			}
		}
		return ""
	}

	var importMeter, exportMeter *MeterInfo
	for _, meterPoint := range property.ElectricityMeterPoints {
		if len(meterPoint.Meters) < 1 {
			continue
		}

		tariffCode := meterPoint.Agreements[len(meterPoint.Agreements)-1].TariffCode
		productCode := findProductCode(tariffCode)

		if meterPoint.IsExport {
			exportMeter = &MeterInfo{
				ProductCode:  productCode,
				TariffCode:   tariffCode,
				SerialNumber: meterPoint.Meters[0].SerialNumber,
				Mpan:         meterPoint.Mpan,
			}
		} else {
			importMeter = &MeterInfo{
				ProductCode:  productCode,
				TariffCode:   tariffCode,
				SerialNumber: meterPoint.Meters[0].SerialNumber,
				Mpan:         meterPoint.Mpan,
			}
		}
	}

	return importMeter, exportMeter, nil
}

// GetLastReading fetches the start date time of the last reading from the Octopus API.
func (s *OctopusService) GetLastReading(meter *MeterInfo) (time.Time, float64, error) {
	orderBy := "-period"
	params := electricity_meter_points.NewListConsumptionForAnElectricityMeterParams().
		WithMpan(meter.Mpan).
		WithSerialNumber(meter.SerialNumber).
		WithOrderBy(&orderBy)

	response, err := s.Client.ElectricityMeterPoints.ListConsumptionForAnElectricityMeter(params, nil)
	if err != nil {
		return time.Time{}, 0, err
	}

	if len(response.Payload.Results) == 0 {
		return time.Time{}, 0, nil
	}

	r := response.Payload.Results[0]
	return time.Time(*r.IntervalStart), r.Consumption, nil
}

// FetchTariffs fetches tariff data for the specified parameters.
// FetchTariffs fetches tariff data for the specified parameters with pagination.
func (s *OctopusService) FetchTariffs(productCode, tariffCode string, start, end time.Time) ([]TariffData, error) {
	var allTariffs []TariffData
	pageSize := int64(672) // Fetch two weeks of half-hour slots per page
	page := int64(1)

	params := products.NewListElectricityTariffStandardUnitRatesParams().
		WithProductCode(productCode).
		WithTariffCode(tariffCode).
		WithPeriodFrom((*strfmt.DateTime)(&start)).
		WithPeriodTo((*strfmt.DateTime)(&end)).
		WithPageSize(&pageSize)

	for {
		params.WithPage(&page)
		response, err := s.Client.Products.ListElectricityTariffStandardUnitRates(params, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch tariffs: %w", err)
		}

		for _, rate := range response.Payload.Results {
			allTariffs = append(allTariffs, TariffData{
				Rate:      rate.ValueIncVat,
				ValidFrom: (*time.Time)(rate.ValidFrom),
				ValidTo:   (*time.Time)(rate.ValidTo),
			})
		}

		if response.Payload.Next == nil {
			break
		}

		page++
	}

	return allTariffs, nil
}

// Placeholder types for API responses
type UsageRow struct {
	Timestamp                time.Time
	CumulativeImportInverter float64
	CumulativeExportInverter float64
	ImportKWh                float64
	ExportKWh                float64
	ImportPrice              float64
	ExportPrice              float64
}

type MeterInfo struct {
	ProductCode  string
	TariffCode   string
	SerialNumber string
	Mpan         string
}

type TariffData struct {
	Rate      float64
	ValidFrom *time.Time
	ValidTo   *time.Time
}
