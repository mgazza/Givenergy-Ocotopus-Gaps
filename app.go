// main.go
package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	"git.sr.ht/~mariusor/cache"
)

// Config contains configuration for the application.
type Config struct {
	APIKey         string
	GivAPIKey      string
	AccountID      string
	SerialNumber   string
	OutputCSV      string
	CacheDirectory string
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
	ExportMeter     *MeterInfo
	CollectionStart time.Time
}

func NewApp(config *Config) *App {
	rt := http.DefaultTransport

	if config.CacheDirectory != "disable" {
		cacheDir := config.CacheDirectory
		if cacheDir == "" {
			cacheDir = os.TempDir()
		}
		//rt = cache.ForDuration(24*time.Hour, http.DefaultTransport, cache.FS(cacheDir))
		rt = cache.Shared(http.DefaultTransport, cache.FS(cacheDir))

		log.Printf("HTTP caching enabled in directory: %s", cacheDir)
	} else {
		log.Println("HTTP caching disabled")
	}

	// Initialize services
	givService := NewGivEnergyService(rt, config.GivAPIKey)
	octopusService := NewOctopusService(rt, config.APIKey)

	// Fetch meter and tariff details
	importMeter, exportMeter, err := octopusService.GetMetersAndTariff(config.AccountID)
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

	return &App{
		Config:          config,
		HTTPClient:      &http.Client{Transport: rt},
		GivService:      givService,
		OctopusService:  octopusService,
		ImportMeter:     importMeter,
		ExportMeter:     exportMeter,
		CollectionStart: collectionStart,
	}
}

func (app *App) Run() error {
	log.Println("Starting application...")
	log.Printf("Getting GivEnergy inverter data for %s %s\n", app.CollectionStart.Format(time.RFC3339), app.Config.EndTime.Format(time.RFC3339))

	// Fetch GivEnergy data
	givData, err := app.GivService.FetchHalfHourlyInverterData(app.Config.SerialNumber, app.CollectionStart, app.Config.EndTime.UTC())
	if err != nil {
		return fmt.Errorf("failed to fetch GivEnergy data: %w", err)
	}
	log.Printf("Fetched %d GivEnergy records", len(givData))

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

func writeCSV(filename string, data []*UsageRow) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"Timestamp",
		"Cumulative_Import",
		"Cumulative_Export",
		"Import_KWh",
		"Export_KWh",
		"Import_Price",
		"Export_Price",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, row := range data {
		record := []string{
			row.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%.4f", row.CumulativeImportInverter),
			fmt.Sprintf("%.4f", row.CumulativeExportInverter),
			fmt.Sprintf("%.4f", row.ImportKWh),
			fmt.Sprintf("%.4f", row.ExportKWh),
			fmt.Sprintf("%.4f", row.ImportPrice),
			fmt.Sprintf("%.4f", row.ExportPrice),
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

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
