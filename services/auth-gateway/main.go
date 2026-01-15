package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

var (
	db        *sql.DB
	jwtSecret []byte
	templates *template.Template
)

type User struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	LastLogin    time.Time `json:"last_login,omitempty"`
}

type Session struct {
	ID        string    `json:"id"`
	UserID    int       `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
}

type Claims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type ValidationResponse struct {
	Valid    bool   `json:"valid"`
	UserID   int    `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"`
	Error    string `json:"error,omitempty"`
}

func main() {
	initDB()
	initJWTSecret()
	initTemplates()
	createTables()
	createDefaultAdmin()

	// Public routes
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/login", handleLogin)
	http.HandleFunc("/logout", handleLogout)
	http.HandleFunc("/register", handleRegister)
	
	// API routes for services
	http.HandleFunc("/api/login", handleAPILogin)
	http.HandleFunc("/api/logout", handleAPILogout)
	http.HandleFunc("/api/register", handleAPIRegister)
	http.HandleFunc("/api/validate", handleValidate)
	http.HandleFunc("/api/refresh", handleRefresh)
	
	// Protected API routes
	http.HandleFunc("/api/users", handleUsers)
	http.HandleFunc("/api/users/", handleUserByID)
	http.HandleFunc("/api/sessions", handleSessions)
	http.HandleFunc("/api/me", handleMe)
	http.HandleFunc("/api/change-password", handleChangePassword)
	
	// Admin pages
	http.HandleFunc("/admin", handleAdmin)
	http.HandleFunc("/admin/users", handleAdminUsers)
	http.HandleFunc("/admin/sessions", handleAdminSessions)
	
	// Health endpoints
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/ready", handleReady)

	log.Println("Auth-gateway v3 starting on :8080")
	log.Println("JWT protection enabled for all HolmOS services")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func initDB() {
	host := getEnv("DB_HOST", "postgres.holm.svc.cluster.local")
	user := getEnv("DB_USER", "postgres")
	password := getEnv("DB_PASSWORD", "postgres")
	dbname := getEnv("DB_NAME", "holm")

	connStr := fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable",
		host, user, password, dbname)

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	if err = db.Ping(); err != nil {
		log.Fatal("Failed to ping database:", err)
	}
	log.Println("Database connection established")
}

func initJWTSecret() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		b := make([]byte, 32)
		rand.Read(b)
		secret = base64.StdEncoding.EncodeToString(b)
		log.Println("Generated new JWT secret")
	}
	jwtSecret = []byte(secret)
}

func initTemplates() {
	templates = template.Must(template.New("").Parse(templatesHTML))
}

func createTables() {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS auth_users (
			id SERIAL PRIMARY KEY,
			username VARCHAR(255) UNIQUE NOT NULL,
			email VARCHAR(255) UNIQUE,
			password_hash VARCHAR(255) NOT NULL,
			role VARCHAR(50) DEFAULT 'user',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_login TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS auth_sessions (
			id VARCHAR(255) PRIMARY KEY,
			user_id INTEGER REFERENCES auth_users(id) ON DELETE CASCADE,
			token TEXT NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			ip VARCHAR(50),
			user_agent TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON auth_sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON auth_sessions(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_users_email ON auth_users(email)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			log.Printf("Table creation query failed: %v", err)
		}
	}
	log.Println("Database tables initialized")
}

func createDefaultAdmin() {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM auth_users WHERE username = 'admin'").Scan(&count)
	if count > 0 {
		return
	}

	password := getEnv("ADMIN_PASSWORD", "admin123")
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	_, err := db.Exec(
		"INSERT INTO auth_users (username, email, password_hash, role) VALUES ($1, $2, $3, $4)",
		"admin", "admin@holm.local", string(hash), "admin",
	)
	if err != nil {
		log.Printf("Failed to create admin user: %v", err)
	} else {
		log.Println("Created default admin user")
	}
}

func generateToken(user *User, duration time.Duration) (string, error) {
	claims := &Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "holmos-auth",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func validateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("invalid token")
}

func createSession(user *User, r *http.Request) (*Session, string, error) {
	accessToken, err := generateToken(user, 15*time.Minute)
	if err != nil {
		return nil, "", err
	}

	refreshToken, err := generateToken(user, 7*24*time.Hour)
	if err != nil {
		return nil, "", err
	}

	sessionID := generateSessionID()
	session := &Session{
		ID:        sessionID,
		UserID:    user.ID,
		Token:     refreshToken,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CreatedAt: time.Now(),
		IP:        getClientIP(r),
		UserAgent: r.UserAgent(),
	}

	_, err = db.Exec(
		"INSERT INTO auth_sessions (id, user_id, token, expires_at, ip, user_agent) VALUES ($1, $2, $3, $4, $5, $6)",
		session.ID, session.UserID, session.Token, session.ExpiresAt, session.IP, session.UserAgent,
	)
	if err != nil {
		return nil, "", err
	}

	db.Exec("UPDATE auth_users SET last_login = $1 WHERE id = $2", time.Now(), user.ID)

	return session, accessToken, nil
}

func getClientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		return strings.Split(forwarded, ",")[0]
	}
	return r.RemoteAddr
}

func generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	
	// Check if user is authenticated
	token := extractToken(r)
	if token != "" {
		claims, err := validateToken(token)
		if err == nil {
			if claims.Role == "admin" {
				http.Redirect(w, r, "/admin", http.StatusFound)
			} else {
				http.Redirect(w, r, "/dashboard", http.StatusFound)
			}
			return
		}
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		redirect := r.URL.Query().Get("redirect")
		if redirect == "" {
			redirect = "/"
		}
		data := map[string]interface{}{
			"Redirect": redirect,
			"Error":    "",
		}
		templates.ExecuteTemplate(w, "login", data)
		return
	}

	if r.Method == "POST" {
		r.ParseForm()
		username := r.FormValue("username")
		password := r.FormValue("password")
		redirect := r.FormValue("redirect")
		if redirect == "" {
			redirect = "/"
		}

		user, err := authenticateUser(username, password)
		if err != nil {
			data := map[string]interface{}{
				"Redirect": redirect,
				"Error":    "Invalid username or password",
			}
			templates.ExecuteTemplate(w, "login", data)
			return
		}

		session, accessToken, err := createSession(user, r)
		if err != nil {
			http.Error(w, "Failed to create session", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "holmos_token",
			Value:    accessToken,
			Path:     "/",
			HttpOnly: true,
			Secure:   false,
			MaxAge:   900,
		})
		http.SetCookie(w, &http.Cookie{
			Name:     "holmos_session",
			Value:    session.ID,
			Path:     "/",
			HttpOnly: true,
			Secure:   false,
			MaxAge:   604800,
		})

		http.Redirect(w, r, redirect, http.StatusFound)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("holmos_session")
	if err == nil {
		db.Exec("DELETE FROM auth_sessions WHERE id = $1", cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "holmos_token",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name:   "holmos_session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	http.Redirect(w, r, "/login", http.StatusFound)
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		templates.ExecuteTemplate(w, "register", map[string]interface{}{
			"Error": "",
		})
		return
	}

	if r.Method == "POST" {
		r.ParseForm()
		username := strings.TrimSpace(r.FormValue("username"))
		email := strings.TrimSpace(r.FormValue("email"))
		password := r.FormValue("password")
		confirmPassword := r.FormValue("confirm_password")

		// Validation
		if len(username) < 3 || len(username) > 50 {
			templates.ExecuteTemplate(w, "register", map[string]interface{}{
				"Error": "Username must be between 3 and 50 characters",
			})
			return
		}

		if !isValidUsername(username) {
			templates.ExecuteTemplate(w, "register", map[string]interface{}{
				"Error": "Username can only contain letters, numbers, and underscores",
			})
			return
		}

		if email != "" && !isValidEmail(email) {
			templates.ExecuteTemplate(w, "register", map[string]interface{}{
				"Error": "Please enter a valid email address",
			})
			return
		}

		if len(password) < 6 {
			templates.ExecuteTemplate(w, "register", map[string]interface{}{
				"Error": "Password must be at least 6 characters",
			})
			return
		}

		if password != confirmPassword {
			templates.ExecuteTemplate(w, "register", map[string]interface{}{
				"Error": "Passwords do not match",
			})
			return
		}

		// Check if username exists
		var exists int
		db.QueryRow("SELECT COUNT(*) FROM auth_users WHERE username = $1", username).Scan(&exists)
		if exists > 0 {
			templates.ExecuteTemplate(w, "register", map[string]interface{}{
				"Error": "Username already taken",
			})
			return
		}

		// Check if email exists
		if email != "" {
			db.QueryRow("SELECT COUNT(*) FROM auth_users WHERE email = $1", email).Scan(&exists)
			if exists > 0 {
				templates.ExecuteTemplate(w, "register", map[string]interface{}{
					"Error": "Email already registered",
				})
				return
			}
		}

		// Create user
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}

		var userID int
		err = db.QueryRow(
			"INSERT INTO auth_users (username, email, password_hash, role) VALUES ($1, $2, $3, $4) RETURNING id",
			username, email, string(hash), "user",
		).Scan(&userID)

		if err != nil {
			templates.ExecuteTemplate(w, "register", map[string]interface{}{
				"Error": "Failed to create account: " + err.Error(),
			})
			return
		}

		// Auto-login after registration
		user := &User{ID: userID, Username: username, Email: email, Role: "user"}
		session, accessToken, err := createSession(user, r)
		if err != nil {
			http.Redirect(w, r, "/login?registered=1", http.StatusFound)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "holmos_token",
			Value:    accessToken,
			Path:     "/",
			HttpOnly: true,
			Secure:   false,
			MaxAge:   900,
		})
		http.SetCookie(w, &http.Cookie{
			Name:     "holmos_session",
			Value:    session.ID,
			Path:     "/",
			HttpOnly: true,
			Secure:   false,
			MaxAge:   604800,
		})

		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func isValidUsername(username string) bool {
	match, _ := regexp.MatchString("^[a-zA-Z0-9_]+$", username)
	return match
}

func isValidEmail(email string) bool {
	match, _ := regexp.MatchString("^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$", email)
	return match
}

func handleAPILogin(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	user, err := authenticateUser(req.Username, req.Password)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid credentials"})
		return
	}

	session, accessToken, err := createSession(user, r)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: session.Token,
		ExpiresIn:    900,
		TokenType:    "Bearer",
	})
}

func handleAPILogout(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	token := extractToken(r)
	if token == "" {
		http.Error(w, "No token provided", http.StatusBadRequest)
		return
	}

	claims, err := validateToken(token)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	db.Exec("DELETE FROM auth_sessions WHERE user_id = $1", claims.UserID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
}

func handleAPIRegister(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	// Validation
	if len(req.Username) < 3 || len(req.Username) > 50 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Username must be between 3 and 50 characters"})
		return
	}

	if !isValidUsername(req.Username) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Username can only contain letters, numbers, and underscores"})
		return
	}

	if req.Email != "" && !isValidEmail(req.Email) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid email address"})
		return
	}

	if len(req.Password) < 6 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Password must be at least 6 characters"})
		return
	}

	// Check if username exists
	var exists int
	db.QueryRow("SELECT COUNT(*) FROM auth_users WHERE username = $1", req.Username).Scan(&exists)
	if exists > 0 {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "Username already taken"})
		return
	}

	// Check if email exists
	if req.Email != "" {
		db.QueryRow("SELECT COUNT(*) FROM auth_users WHERE email = $1", req.Email).Scan(&exists)
		if exists > 0 {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"error": "Email already registered"})
			return
		}
	}

	// Create user
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create user"})
		return
	}

	var userID int
	err = db.QueryRow(
		"INSERT INTO auth_users (username, email, password_hash, role) VALUES ($1, $2, $3, $4) RETURNING id",
		req.Username, req.Email, string(hash), "user",
	).Scan(&userID)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create user: " + err.Error()})
		return
	}

	// Create session and return tokens
	user := &User{ID: userID, Username: req.Username, Email: req.Email, Role: "user"}
	session, accessToken, err := createSession(user, r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "User created but failed to generate tokens"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user": map[string]interface{}{
			"id":       userID,
			"username": req.Username,
			"email":    req.Email,
			"role":     "user",
		},
		"tokens": TokenResponse{
			AccessToken:  accessToken,
			RefreshToken: session.Token,
			ExpiresIn:    900,
			TokenType:    "Bearer",
		},
	})
}

func handleValidate(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	w.Header().Set("Content-Type", "application/json")

	token := extractToken(r)
	if token == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ValidationResponse{Valid: false, Error: "No token provided"})
		return
	}

	claims, err := validateToken(token)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ValidationResponse{Valid: false, Error: err.Error()})
		return
	}

	// Set headers that can be used by other services
	w.Header().Set("X-User-ID", fmt.Sprintf("%d", claims.UserID))
	w.Header().Set("X-Username", claims.Username)
	w.Header().Set("X-User-Role", claims.Role)

	json.NewEncoder(w).Encode(ValidationResponse{
		Valid:    true,
		UserID:   claims.UserID,
		Username: claims.Username,
		Role:     claims.Role,
	})
}

func handleRefresh(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	claims, err := validateToken(req.RefreshToken)
	if err != nil {
		http.Error(w, "Invalid refresh token", http.StatusUnauthorized)
		return
	}

	var sessionID string
	err = db.QueryRow("SELECT id FROM auth_sessions WHERE token = $1 AND expires_at > NOW()", req.RefreshToken).Scan(&sessionID)
	if err != nil {
		http.Error(w, "Session expired", http.StatusUnauthorized)
		return
	}

	user, err := getUserByID(claims.UserID)
	if err != nil {
		http.Error(w, "User not found", http.StatusUnauthorized)
		return
	}

	accessToken, err := generateToken(user, 15*time.Minute)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: req.RefreshToken,
		ExpiresIn:    900,
		TokenType:    "Bearer",
	})
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	claims := requireAuth(w, r, "")
	if claims == nil {
		return
	}

	user, err := getUserByID(claims.UserID)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func handleChangePassword(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	claims := requireAuth(w, r, "")
	if claims == nil {
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Verify current password
	var passwordHash string
	err := db.QueryRow("SELECT password_hash FROM auth_users WHERE id = $1", claims.UserID).Scan(&passwordHash)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.CurrentPassword)); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Current password is incorrect"})
		return
	}

	if len(req.NewPassword) < 6 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "New password must be at least 6 characters"})
		return
	}

	newHash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	_, err = db.Exec("UPDATE auth_users SET password_hash = $1 WHERE id = $2", string(newHash), claims.UserID)
	if err != nil {
		http.Error(w, "Failed to update password", http.StatusInternalServerError)
		return
	}

	// Invalidate all sessions
	db.Exec("DELETE FROM auth_sessions WHERE user_id = $1", claims.UserID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "password changed"})
}

func handleUsers(w http.ResponseWriter, r *http.Request) {
	claims := requireAuth(w, r, "admin")
	if claims == nil {
		return
	}

	switch r.Method {
	case "GET":
		rows, err := db.Query("SELECT id, username, email, role, created_at, last_login FROM auth_users ORDER BY id")
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var users []User
		for rows.Next() {
			var u User
			var lastLogin sql.NullTime
			var email sql.NullString
			rows.Scan(&u.ID, &u.Username, &email, &u.Role, &u.CreatedAt, &lastLogin)
			if email.Valid {
				u.Email = email.String
			}
			if lastLogin.Valid {
				u.LastLogin = lastLogin.Time
			}
			users = append(users, u)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)

	case "POST":
		var req struct {
			Username string `json:"username"`
			Email    string `json:"email"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		if req.Role == "" {
			req.Role = "user"
		}

		hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)

		var id int
		err := db.QueryRow(
			"INSERT INTO auth_users (username, email, password_hash, role) VALUES ($1, $2, $3, $4) RETURNING id",
			req.Username, req.Email, string(hash), req.Role,
		).Scan(&id)

		if err != nil {
			http.Error(w, "Failed to create user: "+err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "username": req.Username})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleUserByID(w http.ResponseWriter, r *http.Request) {
	claims := requireAuth(w, r, "admin")
	if claims == nil {
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/users/")
	var userID int
	fmt.Sscanf(path, "%d", &userID)

	switch r.Method {
	case "GET":
		user, err := getUserByID(userID)
		if err != nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)

	case "PUT":
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		if req.Password != "" {
			hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
			db.Exec("UPDATE auth_users SET password_hash = $1 WHERE id = $2", string(hash), userID)
		}
		if req.Email != "" {
			db.Exec("UPDATE auth_users SET email = $1 WHERE id = $2", req.Email, userID)
		}
		if req.Role != "" {
			db.Exec("UPDATE auth_users SET role = $1 WHERE id = $2", req.Role, userID)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "updated"})

	case "DELETE":
		_, err := db.Exec("DELETE FROM auth_users WHERE id = $1", userID)
		if err != nil {
			http.Error(w, "Failed to delete user", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleSessions(w http.ResponseWriter, r *http.Request) {
	claims := requireAuth(w, r, "admin")
	if claims == nil {
		return
	}

	if r.Method == "GET" {
		rows, err := db.Query(`
			SELECT s.id, s.user_id, u.username, s.expires_at, s.created_at, s.ip, s.user_agent 
			FROM auth_sessions s 
			JOIN auth_users u ON s.user_id = u.id 
			WHERE s.expires_at > NOW()
			ORDER BY s.created_at DESC
		`)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var sessions []map[string]interface{}
		for rows.Next() {
			var id, ip, userAgent, username string
			var userID int
			var expiresAt, createdAt time.Time
			rows.Scan(&id, &userID, &username, &expiresAt, &createdAt, &ip, &userAgent)
			sessions = append(sessions, map[string]interface{}{
				"id":         id,
				"user_id":    userID,
				"username":   username,
				"expires_at": expiresAt,
				"created_at": createdAt,
				"ip":         ip,
				"user_agent": userAgent,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sessions)
		return
	}

	if r.Method == "DELETE" {
		sessionID := r.URL.Query().Get("id")
		if sessionID != "" {
			db.Exec("DELETE FROM auth_sessions WHERE id = $1", sessionID)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	claims := requireAuthPage(w, r, "admin")
	if claims == nil {
		return
	}
	
	// Get counts for dashboard
	var userCount, sessionCount int
	db.QueryRow("SELECT COUNT(*) FROM auth_users").Scan(&userCount)
	db.QueryRow("SELECT COUNT(*) FROM auth_sessions WHERE expires_at > NOW()").Scan(&sessionCount)
	
	templates.ExecuteTemplate(w, "admin", map[string]interface{}{
		"Username":     claims.Username,
		"UserCount":    userCount,
		"SessionCount": sessionCount,
	})
}

func handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	claims := requireAuthPage(w, r, "admin")
	if claims == nil {
		return
	}

	rows, _ := db.Query("SELECT id, username, email, role, created_at, last_login FROM auth_users ORDER BY id")
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		var lastLogin sql.NullTime
		var email sql.NullString
		rows.Scan(&u.ID, &u.Username, &email, &u.Role, &u.CreatedAt, &lastLogin)
		if email.Valid {
			u.Email = email.String
		}
		if lastLogin.Valid {
			u.LastLogin = lastLogin.Time
		}
		users = append(users, u)
	}

	templates.ExecuteTemplate(w, "admin_users", map[string]interface{}{
		"Username": claims.Username,
		"Users":    users,
	})
}

func handleAdminSessions(w http.ResponseWriter, r *http.Request) {
	claims := requireAuthPage(w, r, "admin")
	if claims == nil {
		return
	}

	rows, _ := db.Query(`
		SELECT s.id, s.user_id, u.username, s.expires_at, s.created_at, s.ip, s.user_agent 
		FROM auth_sessions s 
		JOIN auth_users u ON s.user_id = u.id 
		WHERE s.expires_at > NOW()
		ORDER BY s.created_at DESC
	`)
	defer rows.Close()

	var sessions []map[string]interface{}
	for rows.Next() {
		var id, ip, userAgent, username string
		var userID int
		var expiresAt, createdAt time.Time
		rows.Scan(&id, &userID, &username, &expiresAt, &createdAt, &ip, &userAgent)
		sessions = append(sessions, map[string]interface{}{
			"ID":        id,
			"UserID":    userID,
			"Username":  username,
			"ExpiresAt": expiresAt,
			"CreatedAt": createdAt,
			"IP":        ip,
			"UserAgent": userAgent,
		})
	}

	templates.ExecuteTemplate(w, "admin_sessions", map[string]interface{}{
		"Username": claims.Username,
		"Sessions": sessions,
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func handleReady(w http.ResponseWriter, r *http.Request) {
	if err := db.Ping(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Database unavailable"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ready"))
}

func authenticateUser(username, password string) (*User, error) {
	var user User
	err := db.QueryRow(
		"SELECT id, username, email, password_hash, role, created_at FROM auth_users WHERE username = $1",
		username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.Role, &user.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid password")
	}

	return &user, nil
}

func getUserByID(id int) (*User, error) {
	var user User
	var email sql.NullString
	err := db.QueryRow(
		"SELECT id, username, email, role, created_at FROM auth_users WHERE id = $1", id,
	).Scan(&user.ID, &user.Username, &email, &user.Role, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	if email.Valid {
		user.Email = email.String
	}
	return &user, nil
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	cookie, err := r.Cookie("holmos_token")
	if err == nil {
		return cookie.Value
	}

	return r.URL.Query().Get("token")
}

func requireAuth(w http.ResponseWriter, r *http.Request, requiredRole string) *Claims {
	token := extractToken(r)
	if token == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "No token provided"})
		return nil
	}

	claims, err := validateToken(token)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid token"})
		return nil
	}

	if requiredRole != "" && claims.Role != requiredRole && claims.Role != "admin" {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Insufficient permissions"})
		return nil
	}

	return claims
}

func requireAuthPage(w http.ResponseWriter, r *http.Request, requiredRole string) *Claims {
	token := extractToken(r)
	if token == "" {
		http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusFound)
		return nil
	}

	claims, err := validateToken(token)
	if err != nil {
		http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusFound)
		return nil
	}

	if requiredRole != "" && claims.Role != requiredRole && claims.Role != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return nil
	}

	return claims
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

const templatesHTML = `
{{define "login"}}
<!DOCTYPE html>
<html>
<head>
    <title>HolmOS Login</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .login-container {
            background: rgba(255, 255, 255, 0.95);
            padding: 40px;
            border-radius: 16px;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
            width: 100%;
            max-width: 400px;
        }
        .logo {
            text-align: center;
            margin-bottom: 30px;
        }
        .logo h1 {
            font-size: 28px;
            color: #1a1a2e;
        }
        .logo p {
            color: #666;
            margin-top: 5px;
        }
        .form-group {
            margin-bottom: 20px;
        }
        label {
            display: block;
            margin-bottom: 8px;
            color: #333;
            font-weight: 500;
        }
        input {
            width: 100%;
            padding: 12px 16px;
            border: 2px solid #e0e0e0;
            border-radius: 8px;
            font-size: 16px;
            transition: border-color 0.3s;
        }
        input:focus {
            outline: none;
            border-color: #0f3460;
        }
        button {
            width: 100%;
            padding: 14px;
            background: linear-gradient(135deg, #1a1a2e, #0f3460);
            color: white;
            border: none;
            border-radius: 8px;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: transform 0.2s, box-shadow 0.2s;
        }
        button:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(15, 52, 96, 0.4);
        }
        .error {
            background: #ffe6e6;
            color: #cc0000;
            padding: 12px;
            border-radius: 8px;
            margin-bottom: 20px;
            text-align: center;
        }
        .register-link {
            text-align: center;
            margin-top: 20px;
            color: #666;
        }
        .register-link a {
            color: #0f3460;
            text-decoration: none;
            font-weight: 500;
        }
        .register-link a:hover {
            text-decoration: underline;
        }
    </style>
</head>
<body>
    <div class="login-container">
        <div class="logo">
            <h1>HolmOS</h1>
            <p>Secure Authentication</p>
        </div>
        {{if .Error}}
        <div class="error">{{.Error}}</div>
        {{end}}
        <form method="POST" action="/login">
            <input type="hidden" name="redirect" value="{{.Redirect}}">
            <div class="form-group">
                <label for="username">Username</label>
                <input type="text" id="username" name="username" required autofocus>
            </div>
            <div class="form-group">
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required>
            </div>
            <button type="submit">Sign In</button>
        </form>
        <div class="register-link">
            Don't have an account? <a href="/register">Register here</a>
        </div>
    </div>
</body>
</html>
{{end}}

{{define "register"}}
<!DOCTYPE html>
<html>
<head>
    <title>HolmOS Register</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .register-container {
            background: rgba(255, 255, 255, 0.95);
            padding: 40px;
            border-radius: 16px;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
            width: 100%;
            max-width: 400px;
        }
        .logo {
            text-align: center;
            margin-bottom: 30px;
        }
        .logo h1 {
            font-size: 28px;
            color: #1a1a2e;
        }
        .logo p {
            color: #666;
            margin-top: 5px;
        }
        .form-group {
            margin-bottom: 20px;
        }
        label {
            display: block;
            margin-bottom: 8px;
            color: #333;
            font-weight: 500;
        }
        input {
            width: 100%;
            padding: 12px 16px;
            border: 2px solid #e0e0e0;
            border-radius: 8px;
            font-size: 16px;
            transition: border-color 0.3s;
        }
        input:focus {
            outline: none;
            border-color: #0f3460;
        }
        button {
            width: 100%;
            padding: 14px;
            background: linear-gradient(135deg, #1a1a2e, #0f3460);
            color: white;
            border: none;
            border-radius: 8px;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: transform 0.2s, box-shadow 0.2s;
        }
        button:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(15, 52, 96, 0.4);
        }
        .error {
            background: #ffe6e6;
            color: #cc0000;
            padding: 12px;
            border-radius: 8px;
            margin-bottom: 20px;
            text-align: center;
        }
        .login-link {
            text-align: center;
            margin-top: 20px;
            color: #666;
        }
        .login-link a {
            color: #0f3460;
            text-decoration: none;
            font-weight: 500;
        }
        .login-link a:hover {
            text-decoration: underline;
        }
        .hint {
            font-size: 12px;
            color: #888;
            margin-top: 4px;
        }
    </style>
</head>
<body>
    <div class="register-container">
        <div class="logo">
            <h1>HolmOS</h1>
            <p>Create Account</p>
        </div>
        {{if .Error}}
        <div class="error">{{.Error}}</div>
        {{end}}
        <form method="POST" action="/register">
            <div class="form-group">
                <label for="username">Username</label>
                <input type="text" id="username" name="username" required autofocus pattern="[a-zA-Z0-9_]+" minlength="3" maxlength="50">
                <div class="hint">Letters, numbers, and underscores only</div>
            </div>
            <div class="form-group">
                <label for="email">Email (optional)</label>
                <input type="email" id="email" name="email">
            </div>
            <div class="form-group">
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required minlength="6">
                <div class="hint">At least 6 characters</div>
            </div>
            <div class="form-group">
                <label for="confirm_password">Confirm Password</label>
                <input type="password" id="confirm_password" name="confirm_password" required>
            </div>
            <button type="submit">Create Account</button>
        </form>
        <div class="login-link">
            Already have an account? <a href="/login">Sign in</a>
        </div>
    </div>
</body>
</html>
{{end}}

{{define "admin"}}
<!DOCTYPE html>
<html>
<head>
    <title>HolmOS Admin</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #f5f5f5;
        }
        .header {
            background: linear-gradient(135deg, #1a1a2e, #0f3460);
            color: white;
            padding: 20px;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .header h1 { font-size: 24px; }
        .header a {
            color: white;
            text-decoration: none;
            padding: 8px 16px;
            background: rgba(255,255,255,0.1);
            border-radius: 4px;
        }
        .nav {
            background: white;
            padding: 15px 20px;
            border-bottom: 1px solid #e0e0e0;
        }
        .nav a {
            color: #333;
            text-decoration: none;
            margin-right: 20px;
            padding: 8px 16px;
            border-radius: 4px;
        }
        .nav a:hover { background: #f0f0f0; }
        .content {
            padding: 20px;
            max-width: 1200px;
            margin: 0 auto;
        }
        .card {
            background: white;
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .stats {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .stat-card {
            background: linear-gradient(135deg, #1a1a2e, #0f3460);
            color: white;
            padding: 25px;
            border-radius: 12px;
            text-align: center;
        }
        .stat-card h3 {
            font-size: 36px;
            margin-bottom: 5px;
        }
        .stat-card p {
            opacity: 0.8;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>HolmOS Admin</h1>
        <div>
            <span>{{.Username}}</span>
            <a href="/logout">Logout</a>
        </div>
    </div>
    <div class="nav">
        <a href="/admin">Dashboard</a>
        <a href="/admin/users">Users</a>
        <a href="/admin/sessions">Sessions</a>
    </div>
    <div class="content">
        <div class="stats">
            <div class="stat-card">
                <h3>{{.UserCount}}</h3>
                <p>Total Users</p>
            </div>
            <div class="stat-card">
                <h3>{{.SessionCount}}</h3>
                <p>Active Sessions</p>
            </div>
        </div>
        <div class="card">
            <h2>Authentication Dashboard</h2>
            <p style="margin-top:10px; color:#666;">Welcome to the HolmOS authentication management panel.</p>
        </div>
        <div class="card">
            <h3>API Endpoints</h3>
            <ul style="margin-top:10px; margin-left:20px; line-height: 1.8;">
                <li><code>POST /api/login</code> - User login</li>
                <li><code>POST /api/register</code> - User registration</li>
                <li><code>POST /api/logout</code> - User logout</li>
                <li><code>GET /api/validate</code> - Validate JWT token</li>
                <li><code>POST /api/refresh</code> - Refresh access token</li>
                <li><code>GET /api/me</code> - Get current user info</li>
                <li><code>POST /api/change-password</code> - Change password</li>
            </ul>
        </div>
    </div>
</body>
</html>
{{end}}

{{define "admin_users"}}
<!DOCTYPE html>
<html>
<head>
    <title>User Management - HolmOS</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #f5f5f5;
        }
        .header {
            background: linear-gradient(135deg, #1a1a2e, #0f3460);
            color: white;
            padding: 20px;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .header h1 { font-size: 24px; }
        .header a {
            color: white;
            text-decoration: none;
            padding: 8px 16px;
            background: rgba(255,255,255,0.1);
            border-radius: 4px;
        }
        .nav {
            background: white;
            padding: 15px 20px;
            border-bottom: 1px solid #e0e0e0;
        }
        .nav a {
            color: #333;
            text-decoration: none;
            margin-right: 20px;
            padding: 8px 16px;
            border-radius: 4px;
        }
        .nav a:hover { background: #f0f0f0; }
        .content {
            padding: 20px;
            max-width: 1200px;
            margin: 0 auto;
        }
        .card {
            background: white;
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 15px;
        }
        th, td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #e0e0e0;
        }
        th { background: #f9f9f9; font-weight: 600; }
        .badge {
            display: inline-block;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 12px;
            font-weight: 600;
        }
        .badge-admin { background: #e3f2fd; color: #1565c0; }
        .badge-user { background: #f3e5f5; color: #7b1fa2; }
        .btn {
            padding: 8px 16px;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            font-size: 14px;
        }
        .btn-primary {
            background: #1a1a2e;
            color: white;
        }
        .btn-danger {
            background: #dc3545;
            color: white;
        }
        .modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background: rgba(0,0,0,0.5);
            align-items: center;
            justify-content: center;
        }
        .modal.active { display: flex; }
        .modal-content {
            background: white;
            padding: 30px;
            border-radius: 8px;
            width: 100%;
            max-width: 400px;
        }
        .form-group { margin-bottom: 15px; }
        .form-group label { display: block; margin-bottom: 5px; font-weight: 500; }
        .form-group input, .form-group select {
            width: 100%;
            padding: 10px;
            border: 1px solid #ddd;
            border-radius: 4px;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>HolmOS Admin</h1>
        <div>
            <span>{{.Username}}</span>
            <a href="/logout">Logout</a>
        </div>
    </div>
    <div class="nav">
        <a href="/admin">Dashboard</a>
        <a href="/admin/users">Users</a>
        <a href="/admin/sessions">Sessions</a>
    </div>
    <div class="content">
        <div class="card">
            <div style="display:flex; justify-content:space-between; align-items:center;">
                <h2>User Management</h2>
                <button class="btn btn-primary" onclick="showAddUser()">Add User</button>
            </div>
            <table>
                <thead>
                    <tr>
                        <th>ID</th>
                        <th>Username</th>
                        <th>Email</th>
                        <th>Role</th>
                        <th>Created</th>
                        <th>Last Login</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Users}}
                    <tr>
                        <td>{{.ID}}</td>
                        <td>{{.Username}}</td>
                        <td>{{if .Email}}{{.Email}}{{else}}-{{end}}</td>
                        <td><span class="badge badge-{{.Role}}">{{.Role}}</span></td>
                        <td>{{.CreatedAt.Format "2006-01-02"}}</td>
                        <td>{{if .LastLogin.IsZero}}-{{else}}{{.LastLogin.Format "2006-01-02 15:04"}}{{end}}</td>
                        <td>
                            <button class="btn btn-danger" onclick="deleteUser({{.ID}}, '{{.Username}}')" {{if eq .Username "admin"}}disabled{{end}}>Delete</button>
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    </div>

    <div class="modal" id="addUserModal">
        <div class="modal-content">
            <h3>Add New User</h3>
            <form id="addUserForm" onsubmit="return addUser(event)">
                <div class="form-group">
                    <label>Username</label>
                    <input type="text" name="username" required>
                </div>
                <div class="form-group">
                    <label>Email</label>
                    <input type="email" name="email">
                </div>
                <div class="form-group">
                    <label>Password</label>
                    <input type="password" name="password" required>
                </div>
                <div class="form-group">
                    <label>Role</label>
                    <select name="role">
                        <option value="user">User</option>
                        <option value="admin">Admin</option>
                    </select>
                </div>
                <div style="display:flex; gap:10px; margin-top:20px;">
                    <button type="submit" class="btn btn-primary">Create</button>
                    <button type="button" class="btn" onclick="hideAddUser()">Cancel</button>
                </div>
            </form>
        </div>
    </div>

    <script>
        function showAddUser() {
            document.getElementById('addUserModal').classList.add('active');
        }
        function hideAddUser() {
            document.getElementById('addUserModal').classList.remove('active');
        }
        function getToken() {
            const cookies = document.cookie.split(';');
            for (let c of cookies) {
                const [name, val] = c.trim().split('=');
                if (name === 'holmos_token') return val;
            }
            return '';
        }
        async function addUser(e) {
            e.preventDefault();
            const form = e.target;
            const data = {
                username: form.username.value,
                email: form.email.value,
                password: form.password.value,
                role: form.role.value
            };
            const resp = await fetch('/api/users', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': 'Bearer ' + getToken()
                },
                body: JSON.stringify(data)
            });
            if (resp.ok) {
                location.reload();
            } else {
                const err = await resp.text();
                alert('Error: ' + err);
            }
            return false;
        }
        async function deleteUser(id, username) {
            if (!confirm('Delete user ' + username + '?')) return;
            const resp = await fetch('/api/users/' + id, {
                method: 'DELETE',
                headers: { 'Authorization': 'Bearer ' + getToken() }
            });
            if (resp.ok) {
                location.reload();
            } else {
                alert('Failed to delete user');
            }
        }
    </script>
</body>
</html>
{{end}}

{{define "admin_sessions"}}
<!DOCTYPE html>
<html>
<head>
    <title>Sessions - HolmOS</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #f5f5f5;
        }
        .header {
            background: linear-gradient(135deg, #1a1a2e, #0f3460);
            color: white;
            padding: 20px;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .header h1 { font-size: 24px; }
        .header a {
            color: white;
            text-decoration: none;
            padding: 8px 16px;
            background: rgba(255,255,255,0.1);
            border-radius: 4px;
        }
        .nav {
            background: white;
            padding: 15px 20px;
            border-bottom: 1px solid #e0e0e0;
        }
        .nav a {
            color: #333;
            text-decoration: none;
            margin-right: 20px;
            padding: 8px 16px;
            border-radius: 4px;
        }
        .nav a:hover { background: #f0f0f0; }
        .content {
            padding: 20px;
            max-width: 1200px;
            margin: 0 auto;
        }
        .card {
            background: white;
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 15px;
        }
        th, td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #e0e0e0;
        }
        th { background: #f9f9f9; font-weight: 600; }
        .btn {
            padding: 6px 12px;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            font-size: 13px;
        }
        .btn-danger {
            background: #dc3545;
            color: white;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>HolmOS Admin</h1>
        <div>
            <span>{{.Username}}</span>
            <a href="/logout">Logout</a>
        </div>
    </div>
    <div class="nav">
        <a href="/admin">Dashboard</a>
        <a href="/admin/users">Users</a>
        <a href="/admin/sessions">Sessions</a>
    </div>
    <div class="content">
        <div class="card">
            <h2>Active Sessions</h2>
            <table>
                <thead>
                    <tr>
                        <th>User</th>
                        <th>IP Address</th>
                        <th>Created</th>
                        <th>Expires</th>
                        <th>User Agent</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Sessions}}
                    <tr>
                        <td>{{.Username}}</td>
                        <td>{{.IP}}</td>
                        <td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td>
                        <td>{{.ExpiresAt.Format "2006-01-02 15:04"}}</td>
                        <td style="max-width:200px; overflow:hidden; text-overflow:ellipsis;">{{.UserAgent}}</td>
                        <td>
                            <button class="btn btn-danger" onclick="deleteSession('{{.ID}}')">Revoke</button>
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    </div>
    <script>
        function getToken() {
            const cookies = document.cookie.split(';');
            for (let c of cookies) {
                const [name, val] = c.trim().split('=');
                if (name === 'holmos_token') return val;
            }
            return '';
        }
        async function deleteSession(id) {
            if (!confirm('Revoke this session?')) return;
            const resp = await fetch('/api/sessions?id=' + encodeURIComponent(id), {
                method: 'DELETE',
                headers: { 'Authorization': 'Bearer ' + getToken() }
            });
            if (resp.ok) {
                location.reload();
            } else {
                alert('Failed to revoke session');
            }
        }
    </script>
</body>
</html>
{{end}}
`
