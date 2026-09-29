package httpapi

import (
	"net/http"
	"strconv"

	"licitalens.dev/backend/internal/store"
)

func (s *Server) ingestionRuns(w http.ResponseWriter, r *http.Request) {
	runs, ok := s.store.(store.IngestionRunStore)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"data": []store.IngestionRun{}})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	data, err := runs.RecentIngestionRuns(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ingestion_status_unavailable", "não foi possível consultar o histórico PNCP", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// ingestionStatus reports the freshness of the shared PNCP database. It never
// triggers a provider request: clients only read data already persisted by the
// ingestion job.
func (s *Server) ingestionStatus(w http.ResponseWriter, r *http.Request) {
	response := map[string]any{
		"last_successful_at":   nil,
		"last_attempt_at":      nil,
		"last_attempt_success": nil,
	}
	runs, ok := s.store.(store.IngestionRunStore)
	if !ok {
		writeJSON(w, http.StatusOK, response)
		return
	}
	data, err := runs.RecentIngestionRuns(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ingestion_status_unavailable", "não foi possível consultar a atualização PNCP", nil)
		return
	}
	if len(data) > 0 {
		response["last_attempt_at"] = data[0].FinishedAt
		response["last_attempt_success"] = data[0].Success
	}
	for _, run := range data {
		if run.Success {
			response["last_successful_at"] = run.FinishedAt
			break
		}
	}
	writeJSON(w, http.StatusOK, response)
}
