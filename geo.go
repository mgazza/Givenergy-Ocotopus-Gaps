package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	geo "github.com/mgazza/go-geotogether/client"
	geoops "github.com/mgazza/go-geotogether/client/operations"
)

// GeoTogetherService handles interactions with the Geo Together API.
type GeoTogetherService struct {
	Client *geo.GeoTogetherAPI
}

// NewGeoTogetherService creates a new GeoTogetherService with authentication.
func NewGeoTogetherService(tr http.RoundTripper, username, password string) (*GeoTogetherService, error) {
	cfg := geo.DefaultTransportConfig()
	transport := httptransport.New(cfg.Host, cfg.BasePath, cfg.Schemes)
	transport.Transport = tr
	nc := geo.New(transport, strfmt.Default)

	p := geoops.NewPostUsersserviceV2LoginParams().WithBody(geoops.PostUsersserviceV2LoginBody{
		Identity: username,
		Password: password,
	})
	r, err := nc.Operations.PostUsersserviceV2Login(p, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GeoTogether client: %w", err)
	}

	if !r.IsSuccess() {
		return nil, fmt.Errorf("failed to call GeoTogether login: %v", r.Error())
	}

	transport.DefaultAuthentication = httptransport.BearerToken(r.Payload.AccessToken)

	return &GeoTogetherService{Client: nc}, nil
}

// GetUserSystemRoles retrieves users system roles.
func (s *GeoTogetherService) GetUserSystemID() (string, error) {
	r, err := s.Client.Operations.GetAPIUserapiV2UserDetailSystems(
		geoops.NewGetAPIUserapiV2UserDetailSystemsParams().
			WithSystemDetails(true), nil)
	if err != nil {
		return "", fmt.Errorf("failed to fetch live power data: %w", err)
	}
	if !r.IsSuccess() {
		return "", fmt.Errorf("failed to fetch live power data: %v", r.Error())
	}

	for _, m := range r.Payload.SystemDetails {
		if len(m.Devices) > 0 {
			return m.SystemID, nil
		}
	}
	return "", fmt.Errorf("no systems with devices")
}

func (s *GeoTogetherService) GetSystemReadings(systemID string, startDate time.Time, endDate *time.Time) ([]*geoops.GetEpochserviceV1SystemSystemIDReadingsOKBodyItems0, error) {
	p := geoops.NewGetEpochserviceV1SystemSystemIDReadingsParams().
		WithSystemID(systemID).
		WithStartDate(strfmt.Date(startDate))

	if endDate != nil {
		p = p.WithEndDate(strfmt.Date(*endDate))
	}

	r, err := s.Client.Operations.GetEpochserviceV1SystemSystemIDReadings(p, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch live power data: %w", err)
	}
	if !r.IsSuccess() {
		return nil, fmt.Errorf("failed to fetch live power data: %v", r.Error())
	}

	return r.Payload, nil
}

func (s *GeoTogetherService) PopulateGeoData(usage map[time.Time]*UsageRow, startDate, endDate time.Time) error {
	systemID, err := s.GetUserSystemID()
	if err != nil {
		return fmt.Errorf("getting user system roles: %w", err)
	}

	ed := &endDate
	// Ensure the end date includes at least the full day
	if startDate.Year() == endDate.Year() && startDate.YearDay() == endDate.YearDay() {
		ed1 := endDate.Add(24 * time.Hour) // Move to the next calendar day
		ed = &ed1
		if ed.After(time.Now()) {
			ed = nil
		}
	}

	readings, err := s.GetSystemReadings(systemID, startDate, ed)
	if err != nil {
		return fmt.Errorf("getting system readings: %w", err)
	}

	// ** Step 1: Build Cumulative Energy Map **
	cumulativeEnergy := make(map[time.Time]int64)
	cumulativeGas := make(map[time.Time]int64)
	cumulativeCost := make(map[time.Time]int64)
	cumulativeGasCost := make(map[time.Time]int64)

	var lastCumulativeEnergy int64
	var lastCumulativeGas int64
	var lastCumulativeCost int64
	var lastCumulativeGasCost int64

	count := 0
	for _, readingGroup := range readings {
		for _, reading := range readingGroup.Readings {
			count++
			startTime := time.Unix(int64(readingGroup.StartTimestamp), 0).Local()

			if startTime.Before(startDate) {
				log.Printf("Warning discarded GEO data before start date %s", startTime.Format(time.RFC3339))
				continue
			}

			if startTime.After(endDate) {
				log.Printf("Warning discarded GEO data past end date %s", startTime.Format(time.RFC3339))
				continue
			}

			endTime := startTime.Add(time.Duration(reading.Duration) * time.Second)

			// Store cumulative energy at each timestamp
			switch reading.EnergyType {
			case "IMPORT":
				cumulativeEnergy[endTime] = lastCumulativeEnergy + (reading.EnergyWattHours)
				cumulativeCost[endTime] = lastCumulativeCost + (reading.MilliPenceCost)
				lastCumulativeEnergy = cumulativeEnergy[endTime]
				lastCumulativeCost = cumulativeCost[endTime]
			case "GAS_ENERGY":
				cumulativeGas[endTime] = lastCumulativeGas + (reading.EnergyWattHours)
				cumulativeGasCost[endTime] = lastCumulativeGasCost + (reading.MilliPenceCost)
				lastCumulativeGas = cumulativeGas[endTime]
				lastCumulativeGasCost = cumulativeGasCost[endTime]
			}
		}
	}

	// ** Step 2: Walk Through Data in 30-Minute Slots **
	var prevEnergy int64
	var prevGas int64
	var prevCost int64
	var prevGasCost int64

	for t := startDate.Truncate(30 * time.Minute); t.Before(endDate); t = t.Add(30 * time.Minute) {
		var (
			sumEnergy,
			sumGas,
			sumCost,
			sumGasCost,
			count int64
		)

		// Aggregate all intervals that fall within this 30-minute slot
		for offset := 0; offset < 30; offset += 5 { // Works for 5m, 10m, 15m intervals
			ts := t.Add(time.Duration(offset) * time.Minute)

			if value, exists := cumulativeEnergy[ts]; exists {
				sumEnergy += value - prevEnergy
				prevEnergy = value
			}
			if value, exists := cumulativeGas[ts]; exists {
				sumGas += value - prevGas
				prevGas = value
			}
			if value, exists := cumulativeCost[ts]; exists {
				sumCost += value - prevCost
				prevCost = value
			}
			if value, exists := cumulativeGasCost[ts]; exists {
				sumGasCost += value - prevGasCost
				prevGasCost = value
			}
			count++
		}

		if count == 0 {
			log.Printf("No GEO data for %s, using previous values", t.Format(time.RFC3339))
			continue
		}

		// Ensure the rounded time exists in the usage map
		row, exists := usage[t]
		if !exists {
			rt := UsageRow{
				Timestamp: t,
			}
			row = &rt
			usage[t] = row
		}

		// Assign energy used in the 30-minute window
		row.GEO_ImportWh = float64(sumEnergy)
		row.GEO_ImportGasWh = float64(sumGas)
		row.GEO_ImportMilliPenceCost = sumCost
		row.GEO_ImportGasMilliPenceCost = sumGasCost
	}

	log.Printf("Fetched %d GEO records", count)
	return nil
}
