package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

//go:embed index.html
var content embed.FS

// Configuration
const (
	DBPath             = "./data/users.db"
	HysteriaTrafficAPI = "http://hysteria2-server:8081/traffic"
	HysteriaSecret     = "zx8257686@520" // Should match config.yaml
)

// User model
type User struct {
	ID        int       `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	// Traffic stats (merged from Hysteria)
	Tx int64 `json:"tx"`
	Rx int64 `json:"rx"`
}

var db *sql.DB

func initDB() {
	var err error
	// Ensure data directory exists
	if _, err := os.Stat("./data"); os.IsNotExist(err) {
		os.Mkdir("./data", 0755)
	}

	db, err = sql.Open("sqlite", DBPath)
	if err != nil {
		log.Fatal(err)
	}

	createTableSQL := `CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL,
		enabled BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatal(err)
	}
}

// Hysteria Auth Request
type AuthRequest struct {
	Addr string `json:"addr"`
	Auth string `json:"auth"`
}

// ------ Handlers ------

func authHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Strategy: Auth string is "username:password"
	username, password, ok := parseAuth(req.Auth)
	if !ok {
		// Fallback: Check if auth string matches a password for any user (Legacy/Simple mode)
		// But for multi-user correctness, we should enforce user:pass or look up by token.
		// For this implementation, we enforce "username:password" OR we treat the whole string as password if unique.
		// Let's stick to username:password for clarity.
		http.Error(w, "Invalid auth format. Use 'username:password'", http.StatusUnauthorized)
		return
	}

	var storedPassword string
	var enabled bool
	
	err := db.QueryRow("SELECT password, enabled FROM users WHERE username = ?", username).Scan(&storedPassword, &enabled)
	if err == sql.ErrNoRows {
		http.Error(w, "User not found", http.StatusUnauthorized)
		return
	} else if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if !enabled {
		http.Error(w, "User is disabled", http.StatusForbidden)
		return
	}

	if storedPassword != password {
		http.Error(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	// IMPORTANT: Return JSON response as required by Hysteria2 HTTP auth protocol
	// The "id" field is used by Hysteria for traffic logging
	response := map[string]interface{}{
		"ok": true,
		"id": username,
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func usersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getUsers(w, r)
	case http.MethodPost:
		createUser(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func userDetailHandler(w http.ResponseWriter, r *http.Request) {
	// Simple path parsing /api/users/{id}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	id := parts[3]

	switch r.Method {
	case http.MethodPut:
		updateUser(w, r, id)
	case http.MethodDelete:
		deleteUser(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func getUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, username, password, enabled, created_at FROM users")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Password, &u.Enabled, &u.CreatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}

	// Fetch Traffic Stats from Hysteria
	trafficStats := fetchTrafficStats()
	
	// Merge Stats
	for i := range users {
		if stats, ok := trafficStats[users[i].Username]; ok {
			users[i].Tx = int64(stats["tx"].(float64))
			users[i].Rx = int64(stats["rx"].(float64))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func createUser(w http.ResponseWriter, r *http.Request) {
	var u User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, err := db.Exec("INSERT INTO users (username, password, enabled) VALUES (?, ?, ?)", u.Username, u.Password, 1)
	if err != nil {
		http.Error(w, "Failed to create user (username might indicate duplicate)", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func updateUser(w http.ResponseWriter, r *http.Request, id string) {
	var u User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// If password provided, update it. If enabled status matches, update it.
	// We'll update both for simplicity of this API
	_, err := db.Exec("UPDATE users SET password = ?, enabled = ? WHERE id = ?", u.Password, u.Enabled, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func deleteUser(w http.ResponseWriter, r *http.Request, id string) {
	_, err := db.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// ------ Helpers ------

func fetchTrafficStats() map[string]map[string]interface{} {
	client := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequest("GET", HysteriaTrafficAPI, nil)
	req.Header.Set("Authorization", HysteriaSecret)

	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error fetching traffic stats:", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil
	}

	var result map[string]map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func parseAuth(auth string) (string, string, bool) {
	parts := strings.SplitN(auth, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1], true
	}
	return "", "", false
}

// ------ Main ------

func main() {
	initDB()

	// Static Files (Embedded)
	http.Handle("/", http.FileServer(http.FS(content)))

	// API
	http.HandleFunc("/auth", authHandler)
	http.HandleFunc("/api/users", usersHandler)
	http.HandleFunc("/api/users/", userDetailHandler)

	log.Println("Auth Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
