package main

import (
	"flag"
	"log"
	"os"
	"time"
)

// envOrString returns the environment variable value if set, otherwise returns the default value.
func envOrString(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func parseFlags() *Config {
	apiKey := flag.String("apikey", envOrString("OCTOPUS_API_KEY", ""), "Octopus API key")
	givAPIKey := flag.String("givApikey", envOrString("GIVENERGY_API_KEY", ""), "GivEnergy API key")
	accountID := flag.String("accountID", envOrString("OCTOPUS_ACCOUNT_ID", ""), "Octopus Account ID")
	serial := flag.String("inverterSerial", envOrString("GIVENERGY_SERIAL", ""), "GivEnergy inverter serial number")
	outCSV := flag.String("out", envOrString("OUTPUT_CSV", "output.csv"), "Output CSV file")
	cacheDir := flag.String("cache", envOrString("CACHE_DIR", "disable"), "Directory for HTTP cache ('disable' to disable, empty for temporary directory)")
	startDateTime := flag.String("startDateTime", envOrString("START", ""), "Start date time for data fetching (optional, RFC3339 format)")
	endDateTime := flag.String("endDateTime", envOrString("END", ""), "End date time for data fetching (optional, RFC3339 format)")
	geoUsername := flag.String("geoUser", envOrString("GEO_USER", ""), "Geo Username")
	geoPassword := flag.String("geoPassword", envOrString("GEO_PASSWORD", ""), "Geo Password")
	flag.Parse()

	if *apiKey == "" || *accountID == "" || *serial == "" || *givAPIKey == "" || *geoUsername == "" || *geoPassword == "" {
		log.Fatalf("Required flags missing. Usage: %s -apikey=... -givApikey=... -accountID=... -inverterSerial=... -geoUser=... -geoPassword=...", os.Args[0])
	}

	var parsedStartTime *time.Time
	if *startDateTime != "" {
		parsedTime, err := time.Parse(time.RFC3339, *startDateTime)
		if err != nil {
			log.Fatalf("Invalid startTime format: %v", err)
		}
		parsedStartTime = &parsedTime
	}

	var parsedEndTime time.Time
	if *endDateTime != "" {
		parsedTime, err := time.Parse(time.RFC3339, *endDateTime)
		if err != nil {
			log.Fatalf("Invalid endTime format: %v", err)
		}
		parsedEndTime = parsedTime
	} else {
		parsedEndTime = time.Now()
	}

	return &Config{
		APIKey:         *apiKey,
		GivAPIKey:      *givAPIKey,
		AccountID:      *accountID,
		SerialNumber:   *serial,
		OutputCSV:      *outCSV,
		CacheDirectory: *cacheDir,
		StartTime:      parsedStartTime,
		EndTime:        parsedEndTime,
		GeoUsername:    *geoUsername,
		GeoPassword:    *geoPassword,
	}
}

func main() {
	config := parseFlags()
	app := NewApp(config)

	if err := app.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}
