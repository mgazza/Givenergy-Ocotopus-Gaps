# Octopus Gap Filler

## Overview
Octopus Gap Filler is a tool for identifying and filling gaps in Octopus Energy data using information from GivEnergy and GeoTogether. 
It retrieves historical energy usage and tariff data to provide a more complete dataset for analysis.

## Features
- Fetches half-hourly inverter data from GivEnergy
- Fetches half-hourly import/export data from Octopus
- Fetches half-hourly import/gas data from Geo
- Retrieves meter and tariff information from Octopus Energy
- Computes import/export energy usage and prices
- Outputs data to a CSV file for further analysis

## Installation
### Prerequisites
Ensure you have the following installed:
- Go 1.18 or later

## Configuration
### Environment Variables
The application requires API keys and other details to connect to Octopus Energy, GivEnergy and Geo services. 
You can set them via environment variables or modify `main.go`:

```sh
export OCTOPUS_API_KEY="your_octopus_api_key"
export GIVENERGY_API_KEY="your_givenergy_api_key"
export OCTOPUS_ACCOUNT_ID="your_account_id"
export GIVENERGY_SERIAL="your_inverter_serial_number"
export OUTPUT_CSV="output.csv"
export CACHE_DIR="./cache/"
export START="2024-12-09T00:00:00+00:00"
export END="2024-12-10T00:00:00+00:00"
export GEO_USER="user@example.com"
export GEO_PASSWORD="abdfdcdgfg"

```

## Usage
Run the application with:
```sh
go run main.go
```
This will fetch data from the configured sources and save it to `output.csv`.

## Testing
Run unit tests using:
```sh
go test ./...
```
