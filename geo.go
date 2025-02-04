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

	// ** Store Energy Readings in Local Time **
	energyReadings := make(map[time.Time]int64)
	gasReadings := make(map[time.Time]int64)
	costReadings := make(map[time.Time]int64)
	gasCostReadings := make(map[time.Time]int64)

	for _, readingGroup := range readings {
		timestamp := time.Unix(int64(readingGroup.StartTimestamp), 0).Local() // Convert to local time

		for _, reading := range readingGroup.Readings {
			switch reading.EnergyType {
			case "IMPORT":
				energyReadings[timestamp] += reading.EnergyWattHours
				costReadings[timestamp] += reading.MilliPenceCost
			case "GAS_ENERGY":
				gasReadings[timestamp] += reading.EnergyWattHours
				gasCostReadings[timestamp] += reading.MilliPenceCost
			}
		}
	}

	// ** Aggregate Energy & Cost Readings into 30-Minute Buckets in Local Time **
	for t := startDate.Truncate(30 * time.Minute).Local(); t.Before(endDate); t = t.Add(30 * time.Minute) {
		var sumEnergy, sumGas, sumCost, sumGasCost int64

		for offset := 0; offset < 30; offset += 15 {
			ts := t.Add(time.Duration(offset) * time.Minute).Local() // Ensure lookups use local time
			if value, exists := energyReadings[ts]; exists {
				sumEnergy += value
			}
			if value, exists := gasReadings[ts]; exists {
				sumGas += value
			}
			if value, exists := costReadings[ts]; exists {
				sumCost += value
			}
			if value, exists := gasCostReadings[ts]; exists {
				sumGasCost += value
			}
		}

		// If no data, leave it as nil
		if sumEnergy == 0 && sumGas == 0 {
			log.Printf("No GEO data for %s, leaving as nil", t.Format(time.RFC3339))
			continue
		}

		// Ensure the rounded time exists in the usage map
		row, exists := usage[t]
		if !exists {
			row = &UsageRow{Timestamp: t}
			usage[t] = row
		}

		// Assign energy and cost values for the 30-minute window
		row.GEO_ImportWh = &sumEnergy
		row.GEO_ImportGasWh = &sumGas
		row.GEO_ImportMilliPenceCost = &sumCost
		row.GEO_ImportGasMilliPenceCost = &sumGasCost
	}

	log.Printf("Fetched %d GEO records", len(energyReadings))
	return nil
}
