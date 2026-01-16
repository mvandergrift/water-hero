// cmd/waterhero-ingest/main.go
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

type Config struct {
	DeviceID      string
	Email         string
	SessionCookie string
	QuestDBAddr   string
}

type ReadingPayload struct {
	Readings []Reading `json:"data"`
	Success  bool      `json:"success"`
	Code     int       `json:"code"`
}

type Reading struct {
	P string `json:"p"` // timestamp ms
	U string `json:"u"` // uptime ms
	W string `json:"w"` // device/meter id
	G string `json:"g"` // total gallons
	T string `json:"t"` // temp F
}

type APIRequest struct {
	DeviceID string `json:"device_id"`
	Email    string `json:"email"`
	From     int64  `json:"from"`
	To       int64  `json:"to"`
}

func fetchReadings(cfg Config, from, to time.Time) ([]Reading, error) {
	reqBody := APIRequest{
		DeviceID: cfg.DeviceID,
		Email:    cfg.Email,
		From:     from.UnixMilli(),
		To:       to.UnixMilli(),
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://mywaterhero.net/get/readings", bytes.NewReader(body))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", fmt.Sprintf("connect.sid=%s", cfg.SessionCookie))
	req.Header.Set("Origin", "https://mywaterhero.net")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var payload ReadingPayload
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("parse error: %w, body: %s", err, string(respBody))
	}

	return payload.Readings, nil
}

func sendToQuestDB(cfg Config, readings []Reading) error {
	if len(readings) == 0 {
		return nil
	}

	conn, err := net.Dial("tcp", cfg.QuestDBAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	var buf bytes.Buffer
	for _, r := range readings {
		fmt.Fprintf(&buf, "water_readings,device_id=%s total_gallons=%si,temp_f=%si,uptime=%si %s000000\n",
			r.W, r.G, r.T, r.U, r.P)
	}

	_, err = conn.Write(buf.Bytes())
	return err
}

func backfill(cfg Config, start, end time.Time, chunkSize time.Duration) error {
	current := start
	totalReadings := 0

	for current.Before(end) {
		chunkEnd := current.Add(chunkSize)
		if chunkEnd.After(end) {
			chunkEnd = end
		}

		fmt.Printf("fetching %s to %s... ", current.Format("2006-01-02 15:04"), chunkEnd.Format("2006-01-02 15:04"))

		readings, err := fetchReadings(cfg, current, chunkEnd)
		if err != nil {
			return fmt.Errorf("fetch error for chunk starting %s: %w", current.Format(time.RFC3339), err)
		}

		fmt.Printf("%d readings\n", len(readings))

		if err := sendToQuestDB(cfg, readings); err != nil {
			return fmt.Errorf("questdb error for chunk starting %s: %w", current.Format(time.RFC3339), err)
		}

		totalReadings += len(readings)
		current = chunkEnd

		// Be nice to the API
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Printf("backfill complete: %d total readings\n", totalReadings)
	return nil
}

func main() {
	// Flags
	daysBack := flag.Int("days", 0, "number of days to backfill (0 = just last hour)")
	startDate := flag.String("start", "", "start date for backfill (YYYY-MM-DD)")
	endDate := flag.String("end", "", "end date for backfill (YYYY-MM-DD), defaults to now")
	chunkHours := flag.Int("chunk", 24, "chunk size in hours for backfill requests")
	flag.Parse()

	cfg := Config{
		DeviceID:      os.Getenv("WATERHERO_DEVICE_ID"),
		Email:         os.Getenv("WATERHERO_EMAIL"),
		SessionCookie: os.Getenv("WATERHERO_SESSION"),
		QuestDBAddr:   os.Getenv("QUESTDB_ADDR"),
	}

	if cfg.QuestDBAddr == "" {
		cfg.QuestDBAddr = "localhost:9009"
	}

	if cfg.DeviceID == "" || cfg.Email == "" || cfg.SessionCookie == "" {
		fmt.Fprintln(os.Stderr, "missing required env vars: WATERHERO_DEVICE_ID, WATERHERO_EMAIL, WATERHERO_SESSION")
		os.Exit(1)
	}

	var start, end time.Time
	chunkSize := time.Duration(*chunkHours) * time.Hour

	// Determine time range
	if *startDate != "" {
		var err error
		start, err = time.Parse("2006-01-02", *startDate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid start date: %v\n", err)
			os.Exit(1)
		}

		if *endDate != "" {
			end, err = time.Parse("2006-01-02", *endDate)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid end date: %v\n", err)
				os.Exit(1)
			}
			// End of day
			end = end.Add(24 * time.Hour)
		} else {
			end = time.Now()
		}
	} else if *daysBack > 0 {
		end = time.Now()
		start = end.AddDate(0, 0, -*daysBack)
	} else {
		// Default: last hour
		end = time.Now()
		start = end.Add(-1 * time.Hour)
	}

	fmt.Printf("range: %s to %s (chunk size: %s)\n", start.Format("2006-01-02 15:04"), end.Format("2006-01-02 15:04"), chunkSize)

	if err := backfill(cfg, start, end, chunkSize); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
