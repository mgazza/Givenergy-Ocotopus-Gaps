package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"time"
)

func writeCSV(filename string, data []*UsageRow) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Remove the first row as since we don't have the data for the previous row it's not valid
	data = data[1:]

	header := []string{
		"Timestamp",
		"GE_Cumulative_Import",
		"GE_Cumulative_Export",
		"GE_Import_KWh",
		"GE_Export_KWh",
		"GEO_Import_KWh",
		"OCTO_Import_KWh",
		"Import_Price",
		"Export_Price",
		"GE_Import_PenceCost",
		"GE_Export_PenceCost",
		"GEO_Gas_KWh",
		"GEO_Gas_PenceCost",
		"GEO_Import_PenceCost",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, row := range data {
		// Convert prices to integer representation (pence * 10,000)
		importPriceInt := int64(row.ImportPrice * 10000)
		exportPriceInt := int64(row.ExportPrice * 10000)

		// Convert kWh to integer representation (kWh * 10,000)
		importKWhInt := int64(row.GE_ImportKWh * 10000)
		exportKWhInt := int64(row.GE_ExportKWh * 10000)

		// Compute costs as integer math (total pence * 10,000)
		importCostInt := (importKWhInt * importPriceInt) / 10000
		exportCostInt := (exportKWhInt * exportPriceInt) / 10000

		record := []string{
			row.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%.4f", row.CumulativeImportInverter),
			fmt.Sprintf("%.4f", row.CumulativeExportInverter),
			fmt.Sprintf("%.16f", row.GE_ImportKWh),
			fmt.Sprintf("%.16f", row.GE_ExportKWh),
			fmt.Sprintf("%.16f", float64(row.GEO_ImportWh)/1000),
			fmt.Sprintf("%.4f", row.OCTO_ImportKWh),
			fmt.Sprintf("%.4f", row.ImportPrice),
			fmt.Sprintf("%.4f", row.ExportPrice),
			fmt.Sprintf("%.2f", float64(importCostInt)/10000),
			fmt.Sprintf("%.2f", float64(exportCostInt)/10000),
			fmt.Sprintf("%.4f", float64(row.GEO_ImportGasWh)/1000),
			fmt.Sprintf("%.4f", float64(row.GEO_ImportGasMilliPenceCost)/1000.0),
			fmt.Sprintf("%.4f", float64(row.GEO_ImportMilliPenceCost)/1000.0),
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}
