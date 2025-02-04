// main.go
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"sort"
	"time"
)

// Config contains configuration for the application.
type Config struct {
	APIKey         string
	GivAPIKey      string
	AccountID      string
	SerialNumber   string
	OutputCSV      string
	CacheDirectory string
	GeoUsername    string
	GeoPassword    string
	StartTime      *time.Time
	EndTime        time.Time
}

// App manages application dependencies and logic.
type App struct {
	Config          *Config
	HTTPClient      *http.Client
	GivService      *GivEnergyService
	OctopusService  *OctopusService
	ImportMeter     *MeterInfo
	GasMeter        *MeterInfo
	ExportMeter     *MeterInfo
	CollectionStart time.Time
	GeoService      *GeoTogetherService
}

func NewApp(config *Config) *App {
	rt := http.DefaultTransport

	if config.CacheDirectory != "disable" {
		cacheDir := config.CacheDirectory
		if cacheDir == "" {
			cacheDir = os.TempDir()
		}
		err := os.MkdirAll(cacheDir, 0755)
		if err != nil {
			log.Fatalf("failed to create cache dir: %w", err)
		}

		rt = &CachingRoundTripper{
			UnderlyingTransport: http.DefaultTransport, CacheDir: path.Clean(cacheDir),
		}

		log.Printf("HTTP caching enabled in directory: %s", cacheDir)
	} else {
		log.Println("HTTP caching disabled")
	}

	// Initialize services
	givService := NewGivEnergyService(rt, config.GivAPIKey)
	octopusService := NewOctopusService(rt, config.APIKey)

	// Fetch meter and tariff details
	importMeter, exportMeter, gasMeter, err := octopusService.GetMetersAndTariff(config.AccountID)
	if err != nil {
		log.Fatalf("Failed to get meter and tariff details: %v", err)
	}

	// Determine collection start
	var collectionStart time.Time
	if config.StartTime == nil {
		log.Println("Querying latest reading from Octopus...")
		lastReadingDate, lastReadingValue, err := octopusService.GetLastReading(importMeter)
		if err != nil {
			log.Fatalf("Failed to get last reading: %v", err)
		}
		collectionStart = truncateToMidnight(lastReadingDate.Add(-30 * time.Minute))
		log.Printf("Latest reading %s with value %.4f kWh\n",
			lastReadingDate.Format(time.RFC3339),
			lastReadingValue)
		log.Printf("Beginning query from %s\n",
			collectionStart.Format(time.RFC3339))
	} else {
		collectionStart = *config.StartTime
	}

	geoService, err := NewGeoTogetherService(rt, config.GeoUsername, config.GeoPassword)
	if err != nil {
		log.Fatalf("Failed to initialize GeoTogether service: %v", err)
	}

	return &App{
		Config:          config,
		HTTPClient:      &http.Client{Transport: rt},
		GivService:      givService,
		OctopusService:  octopusService,
		ImportMeter:     importMeter,
		GasMeter:        gasMeter,
		ExportMeter:     exportMeter,
		CollectionStart: collectionStart,
		GeoService:      geoService,
	}
}

func (app *App) Run() error {
	log.Println("Starting application...")
	log.Printf("Using date range %s - %s", app.CollectionStart.Format(time.RFC3339), app.Config.EndTime.Format(time.RFC3339))

	givData := make(map[time.Time]*UsageRow)
	var err error

	// Get data from geo
	log.Println("Getting Octopus data...")
	err = app.OctopusService.GetData(givData, app.ImportMeter, app.CollectionStart, app.Config.EndTime.UTC())
	if err != nil {
		return fmt.Errorf("failed to fetch Ocotopus data: %w", err)
	}

	// Get data from geo
	log.Println("Getting GEO data...")
	err = app.GeoService.PopulateGeoData(givData, app.CollectionStart, app.Config.EndTime.UTC())
	if err != nil {
		return fmt.Errorf("failed to fetch GEO data: %w", err)
	}

	// Fetch GivEnergy data
	log.Printf("Getting GivEnergy inverter data ...")
	err = app.GivService.FetchHalfHourlyInverterData(givData, app.Config.SerialNumber, app.CollectionStart, app.Config.EndTime.Local())
	if err != nil {
		return fmt.Errorf("failed to fetch GivEnergy data: %w", err)
	}

	// Fetch Octopus tariffs for both import and export meters
	importTariffs, err := app.OctopusService.FetchTariffs(app.ImportMeter.ProductCode, app.ImportMeter.TariffCode, app.CollectionStart, app.Config.EndTime.UTC())
	if err != nil {
		return fmt.Errorf("failed to fetch import tariffs: %w", err)
	}
	log.Printf("Fetched %d import tariff records", len(importTariffs))

	exportTariffs, err := app.OctopusService.FetchTariffs(app.ExportMeter.ProductCode, app.ExportMeter.TariffCode, app.CollectionStart, app.Config.EndTime.UTC())
	if err != nil {
		return fmt.Errorf("failed to fetch export tariffs: %w", err)
	}
	log.Printf("Fetched %d export tariff records", len(exportTariffs))

	// Calculate half-hourly costs
	var data []*UsageRow
	for timestamp, row := range givData {
		row.ImportPrice = findRateForTime(timestamp, importTariffs)
		row.ExportPrice = findRateForTime(timestamp, exportTariffs)
		data = append(data, row)
	}

	sort.Slice(data, func(i, j int) bool {
		return data[i].Timestamp.Before(data[j].Timestamp)
	})

	// Write CSV output
	if err := writeCSV(app.Config.OutputCSV, data); err != nil {
		return fmt.Errorf("failed to write CSV: %w", err)
	}
	log.Printf("Wrote CSV to %s", app.Config.OutputCSV)

	return nil
}

func findRateForTime(t time.Time, intervals []TariffData) float64 {
	for _, iv := range intervals {
		// Handle nil Start: treat as before zero time
		startBefore := iv.ValidFrom == nil || !t.Before(*iv.ValidFrom)
		// Handle nil End: treat as after Max(time.Time)
		endAfter := iv.ValidTo == nil || t.Before(*iv.ValidTo)

		if startBefore && endAfter {
			return iv.Rate
		}
	}
	return 0.0
}

func truncateToMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
