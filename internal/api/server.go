package api

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.lumeweb.com/mysql-manager/internal/backup"
	"go.lumeweb.com/mysql-manager/internal/config"
	"go.lumeweb.com/mysql-manager/internal/metrics"
	"go.lumeweb.com/mysql-manager/internal/mysql"
)

type AuthToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	LastUsed  time.Time `json:"last_used"`
}

type Server struct {
	server            *http.Server
	logger            *zap.Logger
	mysqlClient       *mysql.Client
	backupManager     *backup.BackupManager
	metricsCollector  *metrics.Collector
	config            *config.Config
	authTokens        map[string]AuthToken
	tokenMutex        sync.RWMutex
}

func NewServer(
	cfg *config.Config, 
	logger *zap.Logger, 
	mysqlClient *mysql.Client,
	backupManager *backup.BackupManager,
	metricsCollector *metrics.Collector,
) (*Server, error) {
	// Validate API configuration
	if err := validateAPIConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid API configuration: %w", err)
	}

	mux := http.NewServeMux()
	
	s := &Server{
		logger:           logger,
		mysqlClient:      mysqlClient,
		backupManager:    backupManager,
		metricsCollector: metricsCollector,
		config:           cfg,
		authTokens:       make(map[string]AuthToken),
	}

	// Setup routes with enhanced authentication
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/backup", s.authenticatedMiddleware(s.handleBackup))
	mux.HandleFunc("/restore", s.authenticatedMiddleware(s.handleRestore))
	mux.HandleFunc("/status", s.authenticatedMiddleware(s.handleStatus))
	mux.HandleFunc("/metrics", s.authenticatedMiddleware(s.handleMetrics))
	mux.HandleFunc("/health", s.handleHealth) // Health check doesn't require auth
	mux.HandleFunc("/verify", s.authenticatedMiddleware(s.handleVerifyBackup))

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.API.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start token cleanup goroutine
	go s.tokenCleanupRoutine()

	return s, nil
}

// Validate API configuration before server creation
func validateAPIConfig(cfg *config.Config) error {
	if cfg.API.Username == "" {
		return fmt.Errorf("API username is required")
	}
	if cfg.API.Password == "" {
		return fmt.Errorf("API password is required")
	}
	if cfg.API.Port <= 0 || cfg.API.Port > 65535 {
		return fmt.Errorf("invalid API port number")
	}
	return nil
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var loginRequest struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&loginRequest); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Constant time comparison for username and password
	if subtle.ConstantTimeCompare([]byte(loginRequest.Username), []byte(s.config.API.Username)) != 1 ||
	   subtle.ConstantTimeCompare([]byte(loginRequest.Password), []byte(s.config.API.Password)) != 1 {
		// Log failed login attempt
		s.logger.Warn("Failed login attempt", 
			zap.String("username", loginRequest.Username),
			zap.String("ip", r.RemoteAddr),
		)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Generate a secure token
	token := generateSecureToken()
	expiresAt := time.Now().Add(24 * time.Hour)

	s.tokenMutex.Lock()
	s.authTokens[token] = AuthToken{
		Token:     token,
		ExpiresAt: expiresAt,
		LastUsed:  time.Now(),
	}
	s.tokenMutex.Unlock()

	response := map[string]string{
		"token":      token,
		"expires_at": expiresAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) authenticatedMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract token from Authorization header
		token := r.Header.Get("Authorization")
		if token == "" {
			http.Error(w, "Missing authorization token", http.StatusUnauthorized)
			return
		}

		s.tokenMutex.Lock()
		authToken, exists := s.authTokens[token]
		
		// Check token validity and update last used time
		if !exists || time.Now().After(authToken.ExpiresAt) {
			delete(s.authTokens, token)
			s.tokenMutex.Unlock()
			
			s.logger.Warn("Invalid or expired token used", 
				zap.String("token", token),
				zap.String("ip", r.RemoteAddr),
			)
			
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Update last used time
		authToken.LastUsed = time.Now()
		s.authTokens[token] = authToken
		s.tokenMutex.Unlock()

		next.ServeHTTP(w, r)
	}
}

// Periodic cleanup of expired tokens
func (s *Server) tokenCleanupRoutine() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		s.tokenMutex.Lock()
		now := time.Now()
		for token, authToken := range s.authTokens {
			// Remove tokens expired more than 1 hour ago
			if now.Sub(authToken.ExpiresAt) > 1*time.Hour {
				delete(s.authTokens, token)
			}
		}
		s.tokenMutex.Unlock()
	}
}

// Secure token generation with more entropy
func generateSecureToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// Implement other handler methods (handleBackup, handleRestore, etc.)
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	// Backup implementation
	ctx := r.Context()
	if err := s.backupManager.CreateBackup(ctx); err != nil {
		s.logger.Error("Backup failed", zap.Error(err))
		http.Error(w, "Backup failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "backup completed"})
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	var restoreRequest struct {
		BackupKey string `json:"backup_key"`
		Force    bool   `json:"force"`
	}

	if err := json.NewDecoder(r.Body).Decode(&restoreRequest); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Check if restore is needed
	needsRestore, err := s.backupManager.IsRestoreNeeded(ctx)
	if err != nil {
		s.logger.Error("Failed to check restore need", zap.Error(err))
		http.Error(w, "Failed to check restore status", http.StatusInternalServerError)
		return
	}

	if !needsRestore && !restoreRequest.Force {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "restore not needed",
			"message": "MySQL data directory appears valid. Use force=true to override.",
		})
		return
	}

	if err := s.backupManager.RestoreBackup(ctx, restoreRequest.BackupKey); err != nil {
		s.logger.Error("Restore failed", 
			zap.String("backup_key", restoreRequest.BackupKey),
			zap.Error(err),
		)
		http.Error(w, "Restore failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "restore completed"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	// MySQL status implementation
	status, err := s.mysqlClient.GetStatus()
	if err != nil {
		s.logger.Error("Failed to get MySQL status", zap.Error(err))
		http.Error(w, "Failed to retrieve status", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Metrics handler implementation
	metrics, err := s.metricsCollector.GetMetrics()
	if err != nil {
		s.logger.Error("Failed to retrieve metrics", zap.Error(err))
		http.Error(w, "Failed to retrieve metrics", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(metrics)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check MySQL connection
	if err := s.mysqlClient.Ping(); err != nil {
		s.logger.Error("Health check failed - MySQL connection error", zap.Error(err))
		http.Error(w, "MySQL connection error", http.StatusServiceUnavailable)
		return
	}

	// Check backup manager
	if s.backupManager == nil {
		s.logger.Error("Health check failed - Backup manager not initialized")
		http.Error(w, "Backup manager not initialized", http.StatusServiceUnavailable)
		return
	}

	response := map[string]interface{}{
		"status": "healthy",
		"mysql": "connected",
		"timestamp": time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleVerifyBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var verifyRequest struct {
		BackupKey string `json:"backup_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&verifyRequest); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	result, err := s.backupManager.VerifyBackup(ctx, verifyRequest.BackupKey)
	if err != nil {
		s.logger.Error("Backup verification failed",
			zap.String("backup_key", verifyRequest.BackupKey),
			zap.Error(err),
		)
		http.Error(w, fmt.Sprintf("Backup verification failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("Starting API server", 
		zap.Int("port", s.config.API.Port),
	)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("API server failed", zap.Error(err))
		}
	}()

	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Shutting down API server")
	return s.server.Shutdown(ctx)
}
