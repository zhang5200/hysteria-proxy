package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
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

// getPublicHost returns the public host for subscription URLs
func getPublicHost() string {
	if host := os.Getenv("PUBLIC_HOST"); host != "" {
		return host
	}
	return "localhost:8080" // fallback
}

// getPublicProtocol returns the protocol (http/https) for subscription URLs
func getPublicProtocol() string {
	if protocol := os.Getenv("PUBLIC_PROTOCOL"); protocol != "" {
		return protocol
	}
	return "http" // default to http
}

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
	// Traffic limit settings
	TrafficLimit       int64 `json:"traffic_limit"`         // Traffic limit in bytes, 0 = unlimited
	AutoDisableOnLimit bool  `json:"auto_disable_on_limit"` // Auto disable when limit exceeded
}

// Node model (represents a hysteria-proxy instance)
type Node struct {
	ID         int        `json:"id"`
	Name       string     `json:"name"`
	Host       string     `json:"host"`
	Secret     string     `json:"secret"`
	Enabled    bool       `json:"enabled"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSyncAt *time.Time `json:"last_sync_at,omitempty"`
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

	// Create users table
	createUsersTableSQL := `CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL,
		enabled BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err = db.Exec(createUsersTableSQL)
	if err != nil {
		log.Fatal(err)
	}

	// Create nodes table (for managing multiple hysteria-proxy instances)
	createNodesTableSQL := `CREATE TABLE IF NOT EXISTS nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		host TEXT NOT NULL,
		secret TEXT NOT NULL,
		enabled BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_sync_at DATETIME
	);`

	_, err = db.Exec(createNodesTableSQL)
	if err != nil {
		log.Fatal(err)
	}

	// Create traffic_stats table (for storing cumulative traffic data from each node)
	// last_tx/last_rx: 上次从 Hysteria 获取的值（用于检测重启）
	// tx/rx: 累计总流量（持续增加，不会因 Hysteria 重启而清零）
	createTrafficStatsTableSQL := `CREATE TABLE IF NOT EXISTS traffic_stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		node_id INTEGER NOT NULL,
		username TEXT NOT NULL,
		tx INTEGER DEFAULT 0,
		rx INTEGER DEFAULT 0,
		last_tx INTEGER DEFAULT 0,
		last_rx INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(node_id, username),
		FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
	);`

	_, err = db.Exec(createTrafficStatsTableSQL)
	if err != nil {
		log.Fatal(err)
	}

	// Upgrade users table schema - add traffic limit fields if not exist
	// SQLite doesn't have IF NOT EXISTS for ALTER TABLE, so we handle errors gracefully
	upgradeSchema()

	// Initialize admin table
	initAdminTable()

	log.Println("Database initialized successfully")
}

// upgradeSchema ensures all required columns exist in the users table
func upgradeSchema() {
	// Check and add columns one by one
	columns := []struct {
		name       string
		definition string
	}{
		{"traffic_limit", "ALTER TABLE users ADD COLUMN traffic_limit INTEGER DEFAULT 0"},
		{"auto_disable_on_limit", "ALTER TABLE users ADD COLUMN auto_disable_on_limit BOOLEAN DEFAULT 1"},
		{"subscription_token", "ALTER TABLE users ADD COLUMN subscription_token TEXT"},
	}

	for _, col := range columns {
		// Check if column exists
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = ?", col.name).Scan(&count)
		if err != nil {
			log.Printf("Error checking column %s: %v", col.name, err)
			continue
		}

		if count == 0 {
			// Column doesn't exist, add it
			_, err = db.Exec(col.definition)
			if err != nil {
				log.Printf("Warning: Failed to add %s column: %v", col.name, err)
			} else {
				log.Printf("Successfully added %s column", col.name)
			}
		}
	}
}

// Hysteria Auth Request
type AuthRequest struct {
	Addr string `json:"addr"`
	Auth string `json:"auth"`
}

// ------ Handlers ------

func authHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received auth request from %s", r.RemoteAddr)

	if r.Method != http.MethodPost {
		log.Printf("Invalid method: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Failed to decode body: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Strategy: Auth string is "username:password"
	username, password, ok := parseAuth(req.Auth)
	log.Printf("Auth parsed: username=%s, password=***, ok=%v, raw_auth_len=%d", username, ok, len(req.Auth))

	if !ok {
		log.Printf("Invalid auth format. Raw auth: %s", req.Auth)
		http.Error(w, "Invalid auth format. Use 'username:password'", http.StatusUnauthorized)
		return
	}

	var storedPassword string
	var enabled bool
	var trafficLimit int64
	var autoDisable bool

	err := db.QueryRow("SELECT password, enabled, traffic_limit, auto_disable_on_limit FROM users WHERE username = ?", username).Scan(&storedPassword, &enabled, &trafficLimit, &autoDisable)
	if err == sql.ErrNoRows {
		log.Printf("User not found: %s", username)
		http.Error(w, "User not found", http.StatusUnauthorized)
		return
	} else if err != nil {
		log.Printf("Database error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if !enabled {
		log.Printf("User disabled: %s", username)
		http.Error(w, "User is disabled", http.StatusForbidden)
		return
	}

	if storedPassword != password {
		log.Printf("Password mismatch for user: %s. Provided: %s, Stored: %s", username, password, storedPassword)
		http.Error(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	// Check traffic limit
	if trafficLimit > 0 && autoDisable {
		var totalTraffic sql.NullInt64
		err := db.QueryRow(`
			SELECT SUM(tx) + SUM(rx) as total 
			FROM traffic_stats 
			WHERE username = ?
		`, username).Scan(&totalTraffic)

		if err == nil && totalTraffic.Valid && totalTraffic.Int64 >= trafficLimit {
			// Auto-disable user due to traffic limit exceeded
			_, err = db.Exec("UPDATE users SET enabled = 0 WHERE username = ?", username)
			if err != nil {
				log.Printf("Failed to disable user %s: %v", username, err)
			}
			log.Printf("User %s disabled due to traffic limit exceeded: %d/%d bytes", username, totalTraffic.Int64, trafficLimit)
			http.Error(w, "Traffic limit exceeded. Account disabled.", http.StatusForbidden)
			return
		}
	}

	log.Printf("Auth successful for user: %s", username)
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
	// Simple path parsing /api/users/{id} or /api/users/{id}/reset-traffic or /api/users/{id}/subscription or /api/users/{id}/generate-token
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Check if this is a reset-traffic request
	if len(parts) >= 5 && parts[4] == "reset-traffic" {
		resetUserTrafficHandler(w, r)
		return
	}

	// Check if this is a subscription request
	if len(parts) >= 5 && parts[4] == "subscription" {
		getUserSubscriptionHandler(w, r)
		return
	}

	// Check if this is a generate-token request
	if len(parts) >= 5 && parts[4] == "generate-token" {
		generateTokenHandler(w, r)
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
	rows, err := db.Query("SELECT id, username, password, enabled, created_at, traffic_limit, auto_disable_on_limit FROM users")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Password, &u.Enabled, &u.CreatedAt, &u.TrafficLimit, &u.AutoDisableOnLimit); err != nil {
			continue
		}
		users = append(users, u)
	}

	// Fetch aggregated traffic stats from database (across all nodes)
	trafficRows, err := db.Query(`
		SELECT username, SUM(tx) as total_tx, SUM(rx) as total_rx
		FROM traffic_stats
		GROUP BY username
	`)
	if err == nil {
		defer trafficRows.Close()
		trafficStats := make(map[string]map[string]int64)
		for trafficRows.Next() {
			var username string
			var tx, rx int64
			if err := trafficRows.Scan(&username, &tx, &rx); err == nil {
				trafficStats[username] = map[string]int64{"tx": tx, "rx": rx}
			}
		}

		// Merge aggregated stats into users
		for i := range users {
			if stats, ok := trafficStats[users[i].Username]; ok {
				users[i].Tx = stats["tx"]
				users[i].Rx = stats["rx"]
			}
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

	// Set default values if not provided
	if u.TrafficLimit == 0 {
		u.TrafficLimit = 0 // 0 means unlimited
	}
	// AutoDisableOnLimit defaults to true from struct or can be set by client

	_, err := db.Exec("INSERT INTO users (username, password, enabled, traffic_limit, auto_disable_on_limit) VALUES (?, ?, ?, ?, ?)",
		u.Username, u.Password, 1, u.TrafficLimit, u.AutoDisableOnLimit)
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

	// Update all user fields including traffic limit settings
	_, err := db.Exec("UPDATE users SET password = ?, enabled = ?, traffic_limit = ?, auto_disable_on_limit = ? WHERE id = ?",
		u.Password, u.Enabled, u.TrafficLimit, u.AutoDisableOnLimit, id)
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

// ------ Node Management Handlers ------

func nodesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getNodes(w, r)
	case http.MethodPost:
		createNode(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func nodeDetailHandler(w http.ResponseWriter, r *http.Request) {
	// Simple path parsing /api/nodes/{id}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	id := parts[3]

	switch r.Method {
	case http.MethodPut:
		updateNode(w, r, id)
	case http.MethodDelete:
		deleteNode(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func getNodes(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, name, host, secret, enabled, created_at, last_sync_at FROM nodes")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	nodes := []Node{}
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Name, &n.Host, &n.Secret, &n.Enabled, &n.CreatedAt, &n.LastSyncAt); err != nil {
			continue
		}
		nodes = append(nodes, n)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

func createNode(w http.ResponseWriter, r *http.Request) {
	var n Node
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, err := db.Exec("INSERT INTO nodes (name, host, secret, enabled) VALUES (?, ?, ?, ?)",
		n.Name, n.Host, n.Secret, true)
	if err != nil {
		http.Error(w, "Failed to create node (name might be duplicate)", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func updateNode(w http.ResponseWriter, r *http.Request, id string) {
	var n Node
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, err := db.Exec("UPDATE nodes SET name = ?, host = ?, secret = ?, enabled = ? WHERE id = ?",
		n.Name, n.Host, n.Secret, n.Enabled, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func deleteNode(w http.ResponseWriter, r *http.Request, id string) {
	_, err := db.Exec("DELETE FROM nodes WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// ------ Traffic Aggregation Handlers ------

func trafficAggregatedHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Aggregate traffic by username across all nodes
	rows, err := db.Query(`
		SELECT username, SUM(tx) as total_tx, SUM(rx) as total_rx
		FROM traffic_stats
		GROUP BY username
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	result := make(map[string]map[string]int64)
	for rows.Next() {
		var username string
		var tx, rx int64
		if err := rows.Scan(&username, &tx, &rx); err != nil {
			continue
		}
		result[username] = map[string]int64{
			"tx": tx,
			"rx": rx,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func trafficByNodeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get traffic stats grouped by node
	rows, err := db.Query(`
		SELECT n.id, n.name, ts.username, ts.tx, ts.rx, ts.updated_at
		FROM traffic_stats ts
		JOIN nodes n ON ts.node_id = n.id
		ORDER BY n.id, ts.updated_at DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type TrafficEntry struct {
		NodeID    int       `json:"node_id"`
		NodeName  string    `json:"node_name"`
		Username  string    `json:"username"`
		Tx        int64     `json:"tx"`
		Rx        int64     `json:"rx"`
		UpdatedAt time.Time `json:"updated_at"`
	}

	entries := []TrafficEntry{}
	for rows.Next() {
		var e TrafficEntry
		if err := rows.Scan(&e.NodeID, &e.NodeName, &e.Username, &e.Tx, &e.Rx, &e.UpdatedAt); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// ------ Traffic Reset Handler ------

func resetUserTrafficHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract user ID from path: /api/users/{id}/reset-traffic
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	userID := parts[3]

	// Get username
	var username string
	err := db.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username)
	if err == sql.ErrNoRows {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Delete all traffic stats for this user
	_, err = db.Exec("DELETE FROM traffic_stats WHERE username = ?", username)
	if err != nil {
		log.Printf("Error resetting traffic for user %s: %v", username, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Re-enable user if they were disabled due to traffic limit
	_, err = db.Exec("UPDATE users SET enabled = 1 WHERE id = ?", userID)
	if err != nil {
		log.Printf("Error re-enabling user %s: %v", username, err)
	}

	log.Printf("Traffic reset for user %s (ID: %s)", username, userID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Traffic reset successfully"})
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

// fetchTrafficFromNode fetches traffic stats from a specific node
func fetchTrafficFromNode(node Node) map[string]map[string]interface{} {
	client := &http.Client{Timeout: 5 * time.Second}
	url := "http://" + node.Host + "/traffic"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Error creating request for node %s: %v", node.Name, err)
		return nil
	}
	req.Header.Set("Authorization", node.Secret)

	resp, err := client.Do(req)
	if err != nil {
		// Check if it's a timeout error
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline exceeded") {
			log.Printf("Error fetching traffic from node %s (%s): Connection timeout - please verify node address is accessible", node.Name, node.Host)
		} else {
			log.Printf("Error fetching traffic from node %s (%s): %v", node.Name, node.Host, err)
		}
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Node %s (%s) returned status %d", node.Name, node.Host, resp.StatusCode)
		return nil
	}

	var result map[string]map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Error decoding traffic from node %s: %v", node.Name, err)
		return nil
	}
	return result
}

// collectTrafficFromAllNodes periodically collects traffic from all enabled nodes
func collectTrafficFromAllNodes() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Starting traffic collection from all nodes...")

		// Get all enabled nodes
		rows, err := db.Query("SELECT id, name, host, secret FROM nodes WHERE enabled = 1")
		if err != nil {
			log.Printf("Error querying nodes: %v", err)
			continue
		}

		nodes := []Node{}
		for rows.Next() {
			var n Node
			if err := rows.Scan(&n.ID, &n.Name, &n.Host, &n.Secret); err != nil {
				continue
			}
			nodes = append(nodes, n)
		}
		rows.Close()

		if len(nodes) == 0 {
			log.Println("No enabled nodes found")
			continue
		}

		// Collect traffic from each node
		for _, node := range nodes {
			traffic := fetchTrafficFromNode(node)
			if traffic == nil {
				continue
			}

			// Update cumulative traffic data in database (增量累加模式)
			for username, stats := range traffic {
				currentTx := int64(stats["tx"].(float64))
				currentRx := int64(stats["rx"].(float64))

				// Only process if there's actual traffic
				if currentTx > 0 || currentRx > 0 {
					// 获取数据库中的上次值
					var dbTx, dbRx, lastTx, lastRx int64
					err := db.QueryRow(
						"SELECT tx, rx, last_tx, last_rx FROM traffic_stats WHERE node_id = ? AND username = ?",
						node.ID, username,
					).Scan(&dbTx, &dbRx, &lastTx, &lastRx)

					var newTx, newRx int64
					if err == sql.ErrNoRows {
						// 首次记录，直接使用当前值
						newTx = currentTx
						newRx = currentRx
					} else if err != nil {
						log.Printf("Error querying traffic for user %s on node %s: %v", username, node.Name, err)
						continue
					} else {
						// 检测 Hysteria 是否重启（当前值 < 上次值）
						if currentTx < lastTx || currentRx < lastRx {
							// Hysteria 重启了，只累加当前值（新的增量）
							newTx = dbTx + currentTx
							newRx = dbRx + currentRx
							log.Printf("Detected Hysteria restart for user %s on node %s, adding increment: tx=%d, rx=%d",
								username, node.Name, currentTx, currentRx)
						} else {
							// 正常情况，累加增量（当前值 - 上次值）
							deltaTx := currentTx - lastTx
							deltaRx := currentRx - lastRx
							newTx = dbTx + deltaTx
							newRx = dbRx + deltaRx
						}
					}

					// 更新数据库（保存累计值和当前 Hysteria 的值）
					_, err = db.Exec(`
						INSERT INTO traffic_stats (node_id, username, tx, rx, last_tx, last_rx, created_at, updated_at)
						VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
						ON CONFLICT(node_id, username) 
						DO UPDATE SET 
							tx = ?,
							rx = ?,
							last_tx = ?,
							last_rx = ?,
							updated_at = CURRENT_TIMESTAMP
					`, node.ID, username, newTx, newRx, currentTx, currentRx,
						newTx, newRx, currentTx, currentRx)

					if err != nil {
						log.Printf("Error updating traffic for user %s on node %s: %v", username, node.Name, err)
					} else {
						log.Printf("Updated traffic for user %s on node %s: total_tx=%d, total_rx=%d (current: %d/%d)",
							username, node.Name, newTx, newRx, currentTx, currentRx)

						// Check traffic limit and auto-disable if exceeded
						var trafficLimit int64
						var autoDisable bool
						err := db.QueryRow(`
							SELECT traffic_limit, auto_disable_on_limit 
							FROM users 
							WHERE username = ?
						`, username).Scan(&trafficLimit, &autoDisable)

						if err == nil && trafficLimit > 0 && autoDisable {
							// Get total traffic across all nodes
							var totalTraffic sql.NullInt64
							err := db.QueryRow(`
								SELECT SUM(tx) + SUM(rx) as total 
								FROM traffic_stats 
								WHERE username = ?
							`, username).Scan(&totalTraffic)

							if err == nil && totalTraffic.Valid && totalTraffic.Int64 >= trafficLimit {
								_, err = db.Exec("UPDATE users SET enabled = 0 WHERE username = ?", username)
								if err != nil {
									log.Printf("Failed to disable user %s: %v", username, err)
								} else {
									log.Printf("User %s auto-disabled due to traffic limit exceeded: %d/%d bytes",
										username, totalTraffic.Int64, trafficLimit)
								}
							}
						}
					}
				}
			}

			// Update last_sync_at for the node
			_, err := db.Exec("UPDATE nodes SET last_sync_at = ? WHERE id = ?", time.Now(), node.ID)
			if err != nil {
				log.Printf("Error updating last_sync_at for node %s: %v", node.Name, err)
			}
		}

		log.Println("Traffic collection completed")
	}
}

// ------ Subscription Handlers ------

// generateToken generates a unique subscription token
func generateToken() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 32)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Nanosecond)
	}
	return string(b)
}

// getUserSubscriptionHandler returns the subscription URL for a user
func getUserSubscriptionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract user ID from path: /api/users/{id}/subscription
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	userID := parts[3]

	// First check if user exists
	var username string
	err := db.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username)
	if err == sql.ErrNoRows {
		log.Printf("User not found: ID %s", userID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "User not found",
		})
		return
	} else if err != nil {
		log.Printf("Database error when fetching user %s: %v", userID, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Database error: " + err.Error(),
		})
		return
	}

	// Get or generate subscription token
	var token sql.NullString
	err = db.QueryRow("SELECT subscription_token FROM users WHERE id = ?", userID).Scan(&token)
	if err != nil {
		log.Printf("Error fetching subscription_token for user %s: %v", userID, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Failed to fetch subscription token. Database may need migration.",
		})
		return
	}

	// Generate token if it doesn't exist
	if !token.Valid || token.String == "" {
		newToken := generateToken()
		_, err = db.Exec("UPDATE users SET subscription_token = ? WHERE id = ?", newToken, userID)
		if err != nil {
			log.Printf("Failed to generate token for user %s: %v", userID, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Failed to generate token",
			})
			return
		}
		token.String = newToken
		token.Valid = true
		log.Printf("Generated new subscription token for user %s", username)
	}

	// Get host from environment variable or request
	// Priority: PUBLIC_HOST env var > r.Host > fallback
	host := getPublicHost()
	if host == "localhost:8080" && r.Host != "" {
		// If no PUBLIC_HOST is set, try to use r.Host
		host = r.Host
	}

	protocol := getPublicProtocol()
	subscriptionURL := protocol + "://" + host + "/subscription/" + token.String

	log.Printf("Generated subscription URL for user %s (ID: %s): %s", username, userID, subscriptionURL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url": subscriptionURL,
	})
}

// generateTokenHandler generates or regenerates a subscription token for a user
func generateTokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract user ID from path: /api/users/{id}/generate-token
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	userID := parts[3]

	// Check if user exists
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", userID).Scan(&exists)
	if err != nil || !exists {
		log.Printf("User not found for ID %s: err=%v, exists=%v", userID, err, exists)
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Generate new token
	newToken := generateToken()
	log.Printf("Attempting to update subscription token for user ID %s", userID)
	_, err = db.Exec("UPDATE users SET subscription_token = ? WHERE id = ?", newToken, userID)
	if err != nil {
		log.Printf("Failed to update subscription token for user ID %s: %v", userID, err)
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully generated new subscription token for user ID %s", userID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"token": newToken,
	})
}

// serveSubscriptionHandler serves the Clash YAML configuration
func serveSubscriptionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract token from path: /subscription/{token}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	token := parts[2]

	// Get user by token
	var username, password string
	err := db.QueryRow("SELECT username, password FROM users WHERE subscription_token = ? AND enabled = 1", token).Scan(&username, &password)
	if err == sql.ErrNoRows {
		http.Error(w, "Invalid or expired subscription", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Get default port from environment variable
	defaultPort := os.Getenv("NODE_PORT")
	if defaultPort == "" {
		defaultPort = "1443" // default port
	}

	// Get all enabled nodes from database
	rows, err := db.Query("SELECT name, host FROM nodes WHERE enabled = 1")
	if err != nil {
		http.Error(w, "Failed to fetch nodes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type NodeInfo struct {
		Name string
		Host string
	}

	nodes := []NodeInfo{}
	for rows.Next() {
		var n NodeInfo
		if err := rows.Scan(&n.Name, &n.Host); err != nil {
			continue
		}
		nodes = append(nodes, n)
	}

	log.Printf("Using %d nodes from database for user %s (proxy port: %s)", len(nodes), username, defaultPort)

	if len(nodes) == 0 {
		http.Error(w, "No enabled nodes available", http.StatusServiceUnavailable)
		return
	}

	// Generate Clash YAML configuration
	var yamlConfig strings.Builder
	yamlConfig.WriteString("proxies:\n")

	proxyNames := []string{}
	for _, node := range nodes {
		// Extract server IP from host (format: ip or ip:port)
		// The host field stores the management API address (e.g., ip:8081)
		// But for proxy connection, we use NODE_PORT (e.g., 1443)
		hostParts := strings.Split(node.Host, ":")
		serverIP := hostParts[0]

		proxyName := "hysteria2-" + serverIP
		proxyNames = append(proxyNames, proxyName)

		// Password format: username:password
		authPassword := username + ":" + password

		yamlConfig.WriteString(fmt.Sprintf(`  - name: "%s"
    type: hysteria2
    server: %s
    port: %s
    password: "%s"
    skip-cert-verify: true
    
`, proxyName, serverIP, defaultPort, authPassword))
	}

	// Add proxy groups
	yamlConfig.WriteString("proxy-groups:\n")
	yamlConfig.WriteString(`  - name: "代理选择"
    type: select
    proxies:
      - "自动选择"
      - "直连"
`)
	for _, name := range proxyNames {
		yamlConfig.WriteString(fmt.Sprintf("      - \"%s\"\n", name))
	}

	yamlConfig.WriteString("\n  - name: \"自动选择\"\n")
	yamlConfig.WriteString("    type: url-test\n")
	yamlConfig.WriteString("    proxies:\n")
	for _, name := range proxyNames {
		yamlConfig.WriteString(fmt.Sprintf("      - \"%s\"\n", name))
	}
	yamlConfig.WriteString("    url: 'http://www.gstatic.com/generate_204'\n")
	yamlConfig.WriteString("    interval: 300\n\n")

	yamlConfig.WriteString(`  - name: "直连"
    type: select
    proxies:
      - DIRECT

rules:
  # 局域网直连
  - IP-CIDR,127.0.0.0/8,直连
  - IP-CIDR,192.168.0.0/16,直连
  - IP-CIDR,10.0.0.0/8,直连
  - IP-CIDR,172.16.0.0/12,直连
  
  # 中国域名和IP直连
  - DOMAIN-SUFFIX,cn,直连
  - GEOIP,CN,直连
  
  # 其他走代理
  - MATCH,代理选择
`)

	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=clash-config.yaml")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(yamlConfig.String()))
}

// ------ Main ------

func main() {
	initDB()

	// Start periodic traffic collection in background
	go collectTrafficFromAllNodes()
	log.Println("Started periodic traffic collection (every 60 seconds)")

	// Static Files - serve from static directory
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Static Files (Embedded) - serve index.html
	http.Handle("/", http.FileServer(http.FS(content)))

	// API - Authentication
	http.HandleFunc("/auth", authHandler)

	// API - Login
	http.HandleFunc("/api/login", loginHandler)

	// API - Admin User Management
	http.HandleFunc("/api/admin-users", adminUsersHandler)
	http.HandleFunc("/api/admin-users/", adminUserDetailHandler)

	// API - User Management
	http.HandleFunc("/api/users", usersHandler)
	http.HandleFunc("/api/users/", userDetailHandler)

	// API - Node Management
	http.HandleFunc("/api/nodes", nodesHandler)
	http.HandleFunc("/api/nodes/", nodeDetailHandler)

	// API - Traffic Aggregation
	http.HandleFunc("/api/traffic/aggregated", trafficAggregatedHandler)
	http.HandleFunc("/api/traffic/by-node", trafficByNodeHandler)

	// API - Subscription
	http.HandleFunc("/subscription/", serveSubscriptionHandler)

	log.Println("Auth Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
