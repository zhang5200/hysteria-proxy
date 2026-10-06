package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func initMembership() error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS plans (kind TEXT PRIMARY KEY, price_cents INTEGER NOT NULL CHECK(price_cents >= 0))`,
		`INSERT OR IGNORE INTO plans VALUES ('monthly', 1900), ('yearly', 16900)`,
		`CREATE TABLE IF NOT EXISTS redemption_cards (id INTEGER PRIMARY KEY, code TEXT UNIQUE NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('monthly','yearly')), price_cents INTEGER NOT NULL, created_at INTEGER NOT NULL, redeemed_by TEXT, redeemed_at INTEGER, expires_at INTEGER)`,
		`CREATE TABLE IF NOT EXISTS sessions (token TEXT PRIMARY KEY, username TEXT NOT NULL, expires_at INTEGER NOT NULL)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func randomSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func createSession(w http.ResponseWriter, r *http.Request, username string) error {
	token, err := randomSecret()
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO sessions VALUES (?, ?, ?)`, hashPassword(token), username, time.Now().Add(24*time.Hour).Unix())
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: "hysteria_session", Value: token, Path: "/", HttpOnly: true, Secure: r.TLS != nil || getPublicProtocol() == "https", SameSite: http.SameSiteStrictMode, MaxAge: 86400})
	return nil
}
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	if c, err := r.Cookie("hysteria_session"); err == nil {
		db.Exec("DELETE FROM sessions WHERE token = ?", hashPassword(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: "hysteria_session", Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

type Card struct {
	ID         int64  `json:"id"`
	Code       string `json:"code"`
	Kind       string `json:"kind"`
	PriceCents int64  `json:"price_cents"`
	CreatedAt  int64  `json:"created_at"`
	RedeemedBy string `json:"redeemed_by"`
	RedeemedAt int64  `json:"redeemed_at"`
	ExpiresAt  int64  `json:"expires_at"`
}

func readCards(query string, args ...interface{}) ([]Card, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cards := []Card{}
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.Code, &c.Kind, &c.PriceCents, &c.CreatedAt, &c.RedeemedBy, &c.RedeemedAt, &c.ExpiresAt); err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

const cardSelect = `SELECT id, code, kind, price_cents, created_at, COALESCE(redeemed_by,''), COALESCE(redeemed_at,0), COALESCE(expires_at,0) FROM redemption_cards`

func membershipHandler(w http.ResponseWriter, r *http.Request) {
	u, ok := requireRequestUser(w, r)
	if !ok {
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	plans := map[string]int64{}
	rows, err := db.Query("SELECT kind,price_cents FROM plans")
	if err != nil {
		http.Error(w, "读取套餐失败", 500)
		return
	}
	for rows.Next() {
		var k string
		var p int64
		if err = rows.Scan(&k, &p); err != nil {
			rows.Close()
			http.Error(w, "读取套餐失败", 500)
			return
		}
		plans[k] = p
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		http.Error(w, "读取套餐失败", 500)
		return
	}
	var expiry sql.NullInt64
	var userID int64
	var enabled bool
	err = db.QueryRow("SELECT id,expires_at,enabled FROM users WHERE username = ?", u.Username).Scan(&userID, &expiry, &enabled)
	if err != nil && err != sql.ErrNoRows {
		http.Error(w, "读取订阅失败", 500)
		return
	}
	history, err := readCards(cardSelect+" WHERE redeemed_by = ? ORDER BY redeemed_at DESC", u.Username)
	if err != nil {
		http.Error(w, "读取兑换记录失败", 500)
		return
	}
	writeJSON(w, map[string]interface{}{"plans": plans, "expires_at": expiry, "user_id": userID, "enabled": enabled, "history": history, "username": u.Username, "role": u.Role})
}
func cardsHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}
	if r.Method == "GET" {
		cards, err := readCards(cardSelect + " ORDER BY id DESC")
		if err != nil {
			http.Error(w, "读取卡密失败", 500)
			return
		}
		writeJSON(w, cards)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var req struct {
		Kind       string `json:"kind"`
		PriceCents int64  `json:"price_cents"`
		Quantity   int    `json:"quantity"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil || (req.Kind != "monthly" && req.Kind != "yearly") || req.PriceCents < 0 || req.PriceCents > 99999900 || req.Quantity < 1 || req.Quantity > 100 {
		http.Error(w, "请选择月卡或年卡，并填写有效价格及数量（1–100）", 400)
		return
	}
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "生成失败，请重试", 500)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE plans SET price_cents = ? WHERE kind = ?", req.PriceCents, req.Kind); err != nil {
		http.Error(w, "保存价格失败", 500)
		return
	}
	cards := []Card{}
	for i := 0; i < req.Quantity; i++ {
		b := make([]byte, 12)
		if _, err = rand.Read(b); err != nil {
			http.Error(w, "生成失败", 500)
			return
		}
		raw := strings.ToUpper(hex.EncodeToString(b))
		code := fmt.Sprintf("HY-%s-%s-%s-%s", raw[:6], raw[6:12], raw[12:18], raw[18:])
		now := time.Now().Unix()
		res, err := tx.Exec("INSERT INTO redemption_cards(code,kind,price_cents,created_at) VALUES(?,?,?,?)", code, req.Kind, req.PriceCents, now)
		if err != nil {
			http.Error(w, "生成失败，请重试", 500)
			return
		}
		id, _ := res.LastInsertId()
		cards = append(cards, Card{ID: id, Code: code, Kind: req.Kind, PriceCents: req.PriceCents, CreatedAt: now})
	}
	if tx.Commit() != nil {
		http.Error(w, "保存卡密失败", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, cards)
}
func redeemHandler(w http.ResponseWriter, r *http.Request) {
	u, ok := requireRequestUser(w, r)
	if !ok {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
		http.Error(w, "请输入有效卡密", 400)
		return
	}
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "兑换失败，请重试", 500)
		return
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	res, err := tx.Exec("UPDATE redemption_cards SET redeemed_by = ?, redeemed_at = ? WHERE code = ? AND redeemed_by IS NULL", u.Username, now.Unix(), req.Code)
	if err != nil {
		http.Error(w, "兑换繁忙，请重试", 503)
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		http.Error(w, "卡密无效或已被兑换，请检查后重试", 400)
		return
	}
	var expiry sql.NullInt64
	err = tx.QueryRow("SELECT expires_at FROM users WHERE username = ?", u.Username).Scan(&expiry)
	if err == sql.ErrNoRows {
		http.Error(w, "当前账号尚未关联 VPN 用户，请联系管理员", 400)
		return
	}
	if err != nil {
		http.Error(w, "读取订阅失败", 500)
		return
	}
	if !expiry.Valid {
		http.Error(w, "当前账号为不限时账户，无需兑换", 400)
		return
	}
	var kind string
	if tx.QueryRow("SELECT kind FROM redemption_cards WHERE code = ?", req.Code).Scan(&kind) != nil {
		http.Error(w, "读取卡密失败", 500)
		return
	}
	base := now.Unix()
	if expiry.Int64 > base {
		base = expiry.Int64
	}
	days := int64(30)
	if kind == "yearly" {
		days = 365
	}
	expires := base + days*86400
	if _, err = tx.Exec("UPDATE users SET expires_at = ? WHERE username = ?", expires, u.Username); err != nil {
		http.Error(w, "更新订阅失败", 500)
		return
	}
	if _, err = tx.Exec("UPDATE redemption_cards SET expires_at = ? WHERE code = ?", expires, req.Code); err != nil {
		http.Error(w, "保存记录失败", 500)
		return
	}
	if tx.Commit() != nil {
		http.Error(w, "兑换失败，请重试", 500)
		return
	}
	writeJSON(w, map[string]interface{}{"expires_at": expires, "days": days})
}
