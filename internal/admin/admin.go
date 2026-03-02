package admin

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/iSundram/GoTunnel/internal/auth"
	"github.com/iSundram/GoTunnel/internal/logging"
	"github.com/iSundram/GoTunnel/internal/metrics"
	"github.com/iSundram/GoTunnel/internal/registry"
)

// APIResponse is the standard JSON envelope for all admin API responses.
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// createTokenRequest is the expected JSON body for POST /api/v1/tokens.
type createTokenRequest struct {
	UserID     string   `json:"user_id"`
	Scopes     []string `json:"scopes"`
	TTLMinutes int      `json:"ttl_minutes"`
}

// AdminAPI provides the admin REST API handlers.
type AdminAPI struct {
	tokenStore auth.TokenStore
	registry   registry.Registry
	metrics    *metrics.Metrics
	logger     *logging.Logger
}

// NewAdminAPI creates a new AdminAPI instance.
func NewAdminAPI(ts auth.TokenStore, r registry.Registry, m *metrics.Metrics, l *logging.Logger) *AdminAPI {
	return &AdminAPI{
		tokenStore: ts,
		registry:   r,
		metrics:    m,
		logger:     l,
	}
}

// Handler returns an http.Handler with all admin routes registered.
func (a *AdminAPI) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/health", a.handleHealth)
	mux.HandleFunc("/api/v1/tokens", a.requireAdmin(a.handleCreateToken))
	mux.HandleFunc("/api/v1/tunnels", a.requireAdmin(a.handleTunnels))
	mux.HandleFunc("/api/v1/tunnels/", a.requireAdmin(a.handleTunnelByID))

	return mux
}

// requireAdmin is middleware that validates a Bearer token with "admin" scope.
func (a *AdminAPI) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			writeJSON(w, http.StatusUnauthorized, APIResponse{
				Code:    http.StatusUnauthorized,
				Message: "missing or invalid authorization header",
			})
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		token, err := a.tokenStore.ValidateToken(tokenStr)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, APIResponse{
				Code:    http.StatusUnauthorized,
				Message: "invalid token: " + err.Error(),
			})
			return
		}

		hasAdmin := false
		for _, s := range token.Scopes {
			if s == "admin" {
				hasAdmin = true
				break
			}
		}
		if !hasAdmin {
			writeJSON(w, http.StatusForbidden, APIResponse{
				Code:    http.StatusForbidden,
				Message: "insufficient scope: admin required",
			})
			return
		}

		next(w, r)
	}
}

// handleHealth responds with service health status.
func (a *AdminAPI) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, APIResponse{
			Code:    http.StatusMethodNotAllowed,
			Message: "method not allowed",
		})
		return
	}
	writeJSON(w, http.StatusOK, APIResponse{
		Code:    http.StatusOK,
		Message: "ok",
		Data:    map[string]string{"status": "ok"},
	})
}

// handleCreateToken handles POST /api/v1/tokens.
func (a *AdminAPI) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, APIResponse{
			Code:    http.StatusMethodNotAllowed,
			Message: "method not allowed",
		})
		return
	}

	var req createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, APIResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid request body: " + err.Error(),
		})
		return
	}

	if req.UserID == "" {
		writeJSON(w, http.StatusBadRequest, APIResponse{
			Code:    http.StatusBadRequest,
			Message: "user_id is required",
		})
		return
	}

	token, err := a.tokenStore.CreateToken(req.UserID, req.Scopes, req.TTLMinutes)
	if err != nil {
		a.logger.Error("failed to create token", "error", err)
		writeJSON(w, http.StatusInternalServerError, APIResponse{
			Code:    http.StatusInternalServerError,
			Message: "failed to create token: " + err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusCreated, APIResponse{
		Code:    http.StatusCreated,
		Message: "token created",
		Data:    token,
	})
}

// handleTunnels handles GET /api/v1/tunnels.
func (a *AdminAPI) handleTunnels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, APIResponse{
			Code:    http.StatusMethodNotAllowed,
			Message: "method not allowed",
		})
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID != "" {
		tunnels, err := a.registry.GetByOwner(userID)
		if err != nil {
			a.logger.Error("failed to list tunnels by owner", "error", err)
			writeJSON(w, http.StatusInternalServerError, APIResponse{
				Code:    http.StatusInternalServerError,
				Message: "failed to list tunnels: " + err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, APIResponse{
			Code:    http.StatusOK,
			Message: "tunnels listed",
			Data:    tunnels,
		})
		return
	}

	tunnels := a.registry.List()
	writeJSON(w, http.StatusOK, APIResponse{
		Code:    http.StatusOK,
		Message: "tunnels listed",
		Data:    tunnels,
	})
}

// handleTunnelByID handles DELETE /api/v1/tunnels/{id}.
func (a *AdminAPI) handleTunnelByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, APIResponse{
			Code:    http.StatusMethodNotAllowed,
			Message: "method not allowed",
		})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/tunnels/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, APIResponse{
			Code:    http.StatusBadRequest,
			Message: "tunnel id is required",
		})
		return
	}

	if err := a.registry.Unregister(id); err != nil {
		a.logger.Error("failed to delete tunnel", "error", err, "tunnel_id", id)
		writeJSON(w, http.StatusNotFound, APIResponse{
			Code:    http.StatusNotFound,
			Message: "tunnel not found: " + err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, APIResponse{
		Code:    http.StatusOK,
		Message: "tunnel deleted",
	})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, resp APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}
