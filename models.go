package main

import "time"

// Placeholder types for API responses
type UsageRow struct {
	Timestamp                   time.Time
	CumulativeImportInverter    float64
	CumulativeExportInverter    float64
	ImportPrice                 float64
	ExportPrice                 float64
	GEO_ImportGasWh             float64
	GEO_ImportWh                float64
	GE_ImportKWh                float64
	GE_ExportKWh                float64
	GEO_ImportMilliPenceCost    int64
	GEO_ImportGasMilliPenceCost int64
	OCTO_ImportKWh              float64
}

type MeterInfo struct {
	ProductCode  string
	TariffCode   string
	SerialNumber string
	Mpan         string // used for both mpan/mprn
}

type TariffData struct {
	Rate      float64
	ValidFrom *time.Time
	ValidTo   *time.Time
}
