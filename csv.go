package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"time"
)

// Helper function to return a pointer to a float64 with optional conversion
func convertInt64(val *int64, divisor float64) *float64 {
	if val != nil {
		newVal := float64(*val) / divisor
		return &newVal
	}
	return nil
}

// Helper function to format float64 values with precision
func formatFloat(val *float64, precision int) string {
	if val != nil {
		formatStr := fmt.Sprintf("%%.%df", precision)
		return fmt.Sprintf(formatStr, *val)
	}
	return "NaN"
}

// Compute the cost using integer math for accuracy
func computeCost(energy *float64, price *float64) string {
	if energy != nil && price != nil {
		energyInt := int64(*energy * (10000))
		priceInt := int64(*price * 10000)
		costInt := (energyInt * priceInt) / 10000
		return fmt.Sprintf("%.2f", float64(costInt)/10000)
	}
	return "NaN"
}

// Write data to a CSV file
func writeCSV(filename string, data []*UsageRow) error {
	if len(data) < 2 {
		return fmt.Errorf("not enough data to write CSV")
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Remove the first row since we don't have the data for the previous row
	data = data[1:]

	header := []string{
		"Timestamp",
		"GE_Cumulative_Import",
		"GE_Cumulative_Export",
		"GE_Import_KWh",
		"GE_Export_KWh",
		"GEO_Import_KWh",
		"OCTO_Import_KWh",
		"OCTO_Export_KWh",
		"GEO_Gas_KWh",
		"Import_Price",
		"Export_Price",
		"GE_Import_PenceCost",
		"GE_Export_PenceCost",
		"GEO_Import_PenceCost",
		"OCTO_Import_PenceCost",
		"OCTO_Export_PenceCost",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, row := range data {
		record := []string{
			row.Timestamp.Format(time.RFC3339),
			formatFloat(row.CumulativeImportInverter, 4),
			formatFloat(row.CumulativeExportInverter, 4),
			formatFloat(row.GE_ImportKWh, 16),
			formatFloat(row.GE_ExportKWh, 16),
			formatFloat(convertInt64(row.GEO_ImportWh, 1000), 16),
			formatFloat(row.OCTO_ImportKWh, 16),
			formatFloat(row.OCTO_ExportKWh, 16),
			formatFloat(convertInt64(row.GEO_ImportGasWh, 1000), 16),
			formatFloat(row.ImportPrice, 4),
			formatFloat(row.ExportPrice, 4),
			computeCost(row.GE_ImportKWh, row.ImportPrice),
			computeCost(row.GE_ExportKWh, row.ExportPrice),
			computeCost(convertInt64(row.GEO_ImportWh, 1000), row.ImportPrice),
			computeCost(row.OCTO_ImportKWh, row.ImportPrice),
			computeCost(row.OCTO_ExportKWh, row.ExportPrice),
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}
