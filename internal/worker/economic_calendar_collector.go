package worker

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/arnabmitra/eth-proxy/internal/fred"
	"github.com/arnabmitra/eth-proxy/internal/repository"
	"github.com/jackc/pgx/v5/pgtype"
)

type EconomicCalendarCollector struct {
	fredClient *fred.Client
	queries    *repository.Queries
	interval   time.Duration
	stop       chan struct{}
}

func NewEconomicCalendarCollector(queries *repository.Queries) *EconomicCalendarCollector {
	apiKey := os.Getenv("FRED_API_KEY")
	
	return &EconomicCalendarCollector{
		fredClient: fred.NewClient(apiKey),
		queries:    queries,
		interval:   24 * time.Hour, // Collect once per day
		stop:       make(chan struct{}),
	}
}

func (c *EconomicCalendarCollector) Start() {
	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		// Run immediately on start
		c.collect()

		for {
			select {
			case <-ticker.C:
				c.collect()
			case <-c.stop:
				return
			}
		}
	}()
}

func (c *EconomicCalendarCollector) Stop() {
	close(c.stop)
}

func (c *EconomicCalendarCollector) collect() {
	startTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Printf("[%s] Starting economic calendar collection from FRED\n", startTime.Format(time.RFC3339))

	// Fetch filtered releases for next 90 days
	releases, err := c.fredClient.GetFilteredReleases(90)
	if err != nil {
		fmt.Printf("Error fetching FRED releases: %v\n", err)
		return
	}

	storedCount := 0
	for _, release := range releases {
		_, err := c.queries.UpsertEconomicRelease(ctx, repository.UpsertEconomicReleaseParams{
			ReleaseID:   int32(release.ReleaseID),
			ReleaseName: release.ReleaseName,
			ReleaseDate: pgtype.Date{Time: release.Date, Valid: true},
			Impact:      release.Impact,
		})

		if err != nil {
			fmt.Printf("Error storing release: %v\n", err)
		} else {
			storedCount++
		}
	}

	fmt.Printf("[%s] Completed economic calendar collection. Stored %d releases in %v\n",
		time.Now().Format(time.RFC3339), storedCount, time.Since(startTime))
}
