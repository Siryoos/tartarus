package olympus

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/persephone"
)

// PersephoneHandlers provides HTTP handlers for Persephone management
type PersephoneHandlers struct {
	scaler *Scaler
}

// NewPersephoneHandlers creates handlers for the given scaler
func NewPersephoneHandlers(scaler *Scaler) *PersephoneHandlers {
	return &PersephoneHandlers{scaler: scaler}
}

// SeasonRequest represents a season creation/update request
type SeasonRequest struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Schedule    persephone.SeasonSchedule `json:"schedule"`
	MinNodes    int                       `json:"min_nodes"`
	MaxNodes    int                       `json:"max_nodes"`
	TargetUtil  float64                   `json:"target_utilization"`
	Prewarming  persephone.PrewarmConfig  `json:"prewarming"`
}

// ForecastRequest represents a forecast query
type ForecastRequest struct {
	Window string `json:"window"` // Duration string like "24h"
}

// ForecastResponse wraps the forecast data
type ForecastResponse struct {
	Forecast    *persephone.Forecast `json:"forecast"`
	GeneratedAt string               `json:"generated_at"`
}

// HandleCreateSeason creates or updates a season
func (h *PersephoneHandlers) HandleCreateSeason(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SeasonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	season := &persephone.Season{
		ID:                req.ID,
		Name:              req.Name,
		Description:       req.Description,
		Schedule:          req.Schedule,
		MinNodes:          req.MinNodes,
		MaxNodes:          req.MaxNodes,
		TargetUtilization: req.TargetUtil,
		Prewarming:        req.Prewarming,
	}

	if err := h.scaler.Persephone.DefineSeason(r.Context(), season); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Register for auto-activation
	h.scaler.RegisterSeason(season)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "created",
		"id":     season.ID,
	})
}

// HandleListSeasons returns all defined seasons
func (h *PersephoneHandlers) HandleListSeasons(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// For BasicSeasonalScaler, we'd need to add a ListSeasons method
	// For now, return current season info
	current, err := h.scaler.Persephone.CurrentSeason(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	seasons := []*persephone.Season{}
	if current != nil {
		seasons = append(seasons, current)
	}

	json.NewEncoder(w).Encode(seasons)
}

// HandleActivateSeason manually activates a season
func (h *PersephoneHandlers) HandleActivateSeason(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract season ID from path: /persephone/seasons/{id}/activate
	path := r.URL.Path
	// Simple path extraction (in production, use a router)
	seasonID := path[len("/persephone/seasons/"):]
	if idx := len(seasonID) - len("/activate"); idx > 0 {
		seasonID = seasonID[:idx]
	}

	if err := h.scaler.Persephone.ApplySeason(r.Context(), seasonID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "activated",
		"season_id": seasonID,
	})
}

// HandleGetForecast returns demand forecast
func (h *PersephoneHandlers) HandleGetForecast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get window parameter
	windowStr := r.URL.Query().Get("window")
	if windowStr == "" {
		windowStr = "24h" // Default to 24 hours
	}

	window, err := time.ParseDuration(windowStr)
	if err != nil {
		http.Error(w, "Invalid window duration", http.StatusBadRequest)
		return
	}

	forecast, err := h.scaler.Persephone.Forecast(r.Context(), window)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := ForecastResponse{
		Forecast:    forecast,
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	json.NewEncoder(w).Encode(response)
}

// HandleGetRecommendations returns capacity recommendations
func (h *PersephoneHandlers) HandleGetRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get current season
	season, err := h.scaler.Persephone.CurrentSeason(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	targetUtil := 0.7 // Default
	if season != nil && season.TargetUtilization > 0 {
		targetUtil = season.TargetUtilization
	}

	recommendation, err := h.scaler.Persephone.RecommendCapacity(r.Context(), targetUtil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(recommendation)
}
