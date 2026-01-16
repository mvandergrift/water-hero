# Water Hero Analytics

A CLI tool to fetch water usage readings from the WaterHero API and store them in QuestDB.

## Features

- Fetches water meter readings from mywaterhero.net
- Stores data in QuestDB using the InfluxDB line protocol
- Supports backfilling historical data with configurable date ranges
- Chunked requests to avoid API rate limits

## Requirements

- Go 1.25+
- QuestDB instance (default: localhost:9009)
- WaterHero account credentials

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `WATERHERO_DEVICE_ID` | Yes | Your WaterHero device ID |
| `WATERHERO_EMAIL` | Yes | Your WaterHero account email |
| `WATERHERO_SESSION` | Yes | Session cookie from mywaterhero.net |
| `QUESTDB_ADDR` | No | QuestDB ILP address (default: localhost:9009) |

## Installation

```bash
go build -o waterhero-ingest ./cmd
```

## Usage

```bash
# Fetch last hour (default)
./waterhero-ingest

# Backfill last 7 days
./waterhero-ingest -days 7

# Backfill specific date range
./waterhero-ingest -start 2024-01-01 -end 2024-01-31

# Custom chunk size (hours per request)
./waterhero-ingest -days 30 -chunk 12
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-days` | 0 | Number of days to backfill (0 = last hour only) |
| `-start` | | Start date for backfill (YYYY-MM-DD) |
| `-end` | now | End date for backfill (YYYY-MM-DD) |
| `-chunk` | 24 | Chunk size in hours for API requests |

## Local Development Environment

The `e2e/` directory contains a Docker Compose environment with QuestDB and Grafana preconfigured for local development and testing.

```bash
cd e2e
docker compose up -d
```

This starts:
- **QuestDB** - Time-series database (ILP: localhost:9009, Web Console: http://localhost:9000)
- **Grafana** - Dashboards for visualizing water usage (http://localhost:3000)

## Data Schema

Readings are stored in the `water_readings` table with the following fields:

- `device_id` (tag) - Meter/device identifier
- `total_gallons` (integer) - Cumulative gallons reading
- `temp_f` (integer) - Water temperature in Fahrenheit
- `uptime` (integer) - Device uptime in milliseconds
- `timestamp` - Reading timestamp
