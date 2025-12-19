package handler

import (
	"context"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/arnabmitra/eth-proxy/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EconomicCalendarHandler struct {
	logger  *slog.Logger
	tmpl    *template.Template
	queries *repository.Queries
}

func NewEconomicCalendarHandler(logger *slog.Logger, tmpl *template.Template, db *pgxpool.Pool) *EconomicCalendarHandler {
	return &EconomicCalendarHandler{
		logger:  logger,
		tmpl:    tmpl,
		queries: repository.New(db),
	}
}

func (h *EconomicCalendarHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := h.tmpl.ExecuteTemplate(w, "economic_calendar.html", nil)
	if err != nil {
		h.logger.Error("Failed to render economic calendar", slog.Any("error", err))
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

type ReleaseView struct {
	ID          string `json:"id"`
	ReleaseID   int32  `json:"release_id"`
	ReleaseName string `json:"release_name"`
	ReleaseDate string `json:"release_date"`
	Impact      string `json:"impact"`
}

func (h *EconomicCalendarHandler) GetThisWeek(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	releases, err := h.queries.GetThisWeekReleases(ctx)
	if err != nil {
		h.logger.Error("Failed to fetch releases", slog.Any("error", err))
		http.Error(w, "Failed to fetch releases", http.StatusInternalServerError)
		return
	}

	views := make([]ReleaseView, 0, len(releases))
	for _, r := range releases {
		views = append(views, ReleaseView{
			ID:          r.ID.String(),
			ReleaseID:   r.ReleaseID,
			ReleaseName: r.ReleaseName,
			ReleaseDate: r.ReleaseDate.Time.Format("2006-01-02"),
			Impact:      r.Impact,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"releases": views,
		"count":    len(views),
	})
}
