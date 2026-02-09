package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
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
	Success  bool   `json:"success"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Message  string `json:"message"`
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RequestUser 当前请求登录用户
type RequestUser struct {
	Username string
	Role     string
}

type ChangePasswordRequest struct {
	NewPassword string `json:"new_password"`
}

const defaultRegisterTrafficLimitBytes int64 = 1000 * 1024 * 1024 * 1024

var usernamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{3,19}$`)

func validateUsername(username string) error {
	if !usernamePattern.MatchString(username) {
		return errors.New("用户名格式不正确：需4-20位，以字母开头，只能包含字母、数字、下划线")
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 8 || len(password) > 32 {
		return errors.New("密码格式不正确：长度需为8-32位")
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		if ch > unicode.MaxASCII {
			return errors.New("密码仅支持英文字符")
		}
		if unicode.IsSpace(ch) {
			return errors.New("密码不能包含空格")
		}

		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		default:
			hasSpecial = true
		}
	}

	if !hasUpper || !hasLower || !hasDigit || !hasSpecial {
		return errors.New("密码需包含大写字母、小写字母、数字和特殊字符")
	}

	return nil
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
		Success:  true,
		Username: req.Username,
		Role:     role,
		Message:  "登录成功",
	})
}

// registerHandler 处理注册请求：创建账号管理用户 + 代理用户
func registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username and password are required", http.StatusBadRequest)
		return
	}
	if err := validateUsername(req.Username); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validatePassword(req.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var adminExists int
	if err := tx.QueryRow("SELECT COUNT(*) FROM admin_users WHERE username = ?", req.Username).Scan(&adminExists); err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if adminExists > 0 {
		http.Error(w, "Username already exists", http.StatusConflict)
		return
	}

	var userExists int
	if err := tx.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", req.Username).Scan(&userExists); err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if userExists > 0 {
		http.Error(w, "Username already exists", http.StatusConflict)
		return
	}

	hashedPassword := hashPassword(req.Password)
	if _, err := tx.Exec("INSERT INTO admin_users (username, password, role) VALUES (?, ?, ?)", req.Username, hashedPassword, "user"); err != nil {
		http.Error(w, "Failed to create account", http.StatusInternalServerError)
		return
	}

	if _, err := tx.Exec(
		"INSERT INTO users (username, password, enabled, traffic_limit, auto_disable_on_limit) VALUES (?, ?, ?, ?, ?)",
		req.Username, req.Password, 1, defaultRegisterTrafficLimitBytes, true,
	); err != nil {
		http.Error(w, "Failed to create default user", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	log.Printf("User registered successfully: %s", req.Username)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "注册成功"})
}

func getRequestUser(r *http.Request) (*RequestUser, error) {
	username := strings.TrimSpace(r.Header.Get("X-Auth-Username"))
	if username == "" {
		return nil, http.ErrNoCookie
	}

	var role string
	err := db.QueryRow("SELECT role FROM admin_users WHERE username = ?", username).Scan(&role)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}

	return &RequestUser{
		Username: username,
		Role:     role,
	}, nil
}

func requireRequestUser(w http.ResponseWriter, r *http.Request) (*RequestUser, bool) {
	reqUser, err := getRequestUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	return reqUser, true
}

func requireAdmin(w http.ResponseWriter, r *http.Request) (*RequestUser, bool) {
	reqUser, ok := requireRequestUser(w, r)
	if !ok {
		return nil, false
	}
	if reqUser.Role != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return nil, false
	}
	return reqUser, true
}

func changePasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reqUser, ok := requireRequestUser(w, r)
	if !ok {
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	req.NewPassword = strings.TrimSpace(req.NewPassword)
	if req.NewPassword == "" {
		http.Error(w, "New password is required", http.StatusBadRequest)
		return
	}
	if err := validatePassword(req.NewPassword); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	hashed := hashPassword(req.NewPassword)
	if _, err := tx.Exec("UPDATE admin_users SET password = ? WHERE username = ?", hashed, reqUser.Username); err != nil {
		http.Error(w, "Failed to update account password", http.StatusInternalServerError)
		return
	}

	if _, err := tx.Exec("UPDATE users SET password = ? WHERE username = ?", req.NewPassword, reqUser.Username); err != nil {
		http.Error(w, "Failed to update proxy password", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "密码修改成功"})
}

// adminUsersHandler 处理管理员用户列表请求
func adminUsersHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}

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
	if _, ok := requireAdmin(w, r); !ok {
		return
	}

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
	u.Username = strings.TrimSpace(u.Username)

	// 验证必填字段
	if u.Username == "" || u.Password == "" {
		http.Error(w, "Username and password are required", http.StatusBadRequest)
		return
	}
	if err := validateUsername(u.Username); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validatePassword(u.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
		if err := validatePassword(u.Password); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

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
