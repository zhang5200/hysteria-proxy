package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// AdminUser 管理员用户模型
type AdminUser struct {
	ID        int       `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"password"` // 存储哈希后的密码
	Role      string    `json:"role"`     // admin 或 user
	CreatedAt time.Time `json:"created_at"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Success bool   `json:"success"`
	Role    string `json:"role"`
	Message string `json:"message"`
}

// hashPassword 对密码进行SHA256哈希
func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// initAdminTable 初始化管理员表
func initAdminTable() {
	// 创建管理员表
	createAdminTableSQL := `CREATE TABLE IF NOT EXISTS admin_users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL,
		role TEXT DEFAULT 'user',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err := db.Exec(createAdminTableSQL)
	if err != nil {
		log.Fatal("Failed to create admin_users table:", err)
	}

	// 检查是否已存在默认管理员账号
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM admin_users WHERE username = ?", "admin").Scan(&count)
	if err != nil {
		log.Printf("Error checking admin user: %v", err)
		return
	}

	// 如果不存在，创建默认管理员账号
	if count == 0 {
		hashedPassword := hashPassword("Zx8257686@520")
		_, err = db.Exec("INSERT INTO admin_users (username, password, role) VALUES (?, ?, ?)",
			"admin", hashedPassword, "admin")
		if err != nil {
			log.Printf("Failed to create default admin user: %v", err)
		} else {
			log.Println("Default admin user created: admin / Zx8257686@520")
		}
	}

	log.Println("Admin table initialized successfully")
}

// loginHandler 处理登录请求
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// 查询用户
	var storedPassword, role string
	err := db.QueryRow("SELECT password, role FROM admin_users WHERE username = ?", req.Username).
		Scan(&storedPassword, &role)

	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LoginResponse{
			Success: false,
			Message: "用户名或密码错误",
		})
		return
	} else if err != nil {
		log.Printf("Database error during login: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// 验证密码
	hashedPassword := hashPassword(req.Password)
	if hashedPassword != storedPassword {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LoginResponse{
			Success: false,
			Message: "用户名或密码错误",
		})
		return
	}

	// 登录成功
	log.Printf("User %s logged in successfully with role: %s", req.Username, role)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{
		Success: true,
		Role:    role,
		Message: "登录成功",
	})
}

// adminUsersHandler 处理管理员用户列表请求
func adminUsersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getAdminUsers(w, r)
	case http.MethodPost:
		createAdminUser(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// adminUserDetailHandler 处理单个管理员用户的操作
func adminUserDetailHandler(w http.ResponseWriter, r *http.Request) {
	// 解析路径 /api/admin-users/{id}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	id := parts[3]

	switch r.Method {
	case http.MethodPut:
		updateAdminUser(w, r, id)
	case http.MethodDelete:
		deleteAdminUser(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getAdminUsers 获取所有管理员用户
func getAdminUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, username, role, created_at FROM admin_users ORDER BY created_at DESC")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	adminUsers := []AdminUser{}
	for rows.Next() {
		var u AdminUser
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt); err != nil {
			continue
		}
		adminUsers = append(adminUsers, u)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(adminUsers)
}

// createAdminUser 创建新的管理员用户
func createAdminUser(w http.ResponseWriter, r *http.Request) {
	var u AdminUser
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 验证必填字段
	if u.Username == "" || u.Password == "" {
		http.Error(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	// 设置默认角色
	if u.Role == "" {
		u.Role = "user"
	}

	// 验证角色
	if u.Role != "admin" && u.Role != "user" {
		http.Error(w, "Invalid role. Must be 'admin' or 'user'", http.StatusBadRequest)
		return
	}

	// 哈希密码
	hashedPassword := hashPassword(u.Password)

	_, err := db.Exec("INSERT INTO admin_users (username, password, role) VALUES (?, ?, ?)",
		u.Username, hashedPassword, u.Role)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "Username already exists", http.StatusConflict)
		} else {
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
		}
		return
	}

	log.Printf("Admin user created: %s (role: %s)", u.Username, u.Role)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "User created successfully"})
}

// updateAdminUser 更新管理员用户
func updateAdminUser(w http.ResponseWriter, r *http.Request, id string) {
	var u AdminUser
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 验证角色
	if u.Role != "" && u.Role != "admin" && u.Role != "user" {
		http.Error(w, "Invalid role. Must be 'admin' or 'user'", http.StatusBadRequest)
		return
	}

	// 如果提供了新密码，则哈希它
	if u.Password != "" {
		hashedPassword := hashPassword(u.Password)
		_, err := db.Exec("UPDATE admin_users SET password = ?, role = ? WHERE id = ?",
			hashedPassword, u.Role, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// 只更新角色
		_, err := db.Exec("UPDATE admin_users SET role = ? WHERE id = ?", u.Role, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	log.Printf("Admin user updated: ID %s", id)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "User updated successfully"})
}

// deleteAdminUser 删除管理员用户
func deleteAdminUser(w http.ResponseWriter, r *http.Request, id string) {
	// 防止删除默认管理员账号
	var username string
	err := db.QueryRow("SELECT username FROM admin_users WHERE id = ?", id).Scan(&username)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if username == "admin" {
		http.Error(w, "Cannot delete default admin user", http.StatusForbidden)
		return
	}

	_, err = db.Exec("DELETE FROM admin_users WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Admin user deleted: %s (ID: %s)", username, id)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "User deleted successfully"})
}
