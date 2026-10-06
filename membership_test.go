package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func membershipTestDB(t *testing.T) {
	t.Helper()
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`CREATE TABLE users(id INTEGER PRIMARY KEY,username TEXT UNIQUE,password TEXT,enabled BOOLEAN DEFAULT 1,traffic_limit INTEGER DEFAULT 0,auto_disable_on_limit BOOLEAN DEFAULT 1,expires_at INTEGER,subscription_token TEXT)`,
		`CREATE TABLE admin_users(id INTEGER PRIMARY KEY,username TEXT UNIQUE,password TEXT,role TEXT)`,
		`INSERT INTO admin_users(username,password,role) VALUES ('admin','unused','admin'),('member','unused','user'),('other','unused','user')`,
		`INSERT INTO users(username,password,expires_at,subscription_token) VALUES ('member','Pass123!',0,'test-subscription'),('other','Pass123!',0,'other-subscription')`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err = initMembership(); err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{"admin", "member", "other"} {
		if _, err = db.Exec("INSERT INTO sessions VALUES(?,?,?)", hashPassword(user+"-token"), user, time.Now().Add(time.Hour).Unix()); err != nil {
			t.Fatal(err)
		}
	}
}
func membershipRequest(handler http.HandlerFunc, method, user, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/", bytes.NewBufferString(body))
	if user != "" {
		r.AddCookie(&http.Cookie{Name: "hysteria_session", Value: user + "-token"})
	}
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}
func generateTestCard(t *testing.T, kind string) Card {
	t.Helper()
	w := membershipRequest(cardsHandler, "POST", "admin", fmt.Sprintf(`{"kind":%q,"price_cents":2990,"quantity":1}`, kind))
	if w.Code != 201 {
		t.Fatalf("generate: %d %s", w.Code, w.Body)
	}
	var cards []Card
	if err := json.Unmarshal(w.Body.Bytes(), &cards); err != nil {
		t.Fatal(err)
	}
	return cards[0]
}
func TestCardGenerationAndPermissions(t *testing.T) {
	membershipTestDB(t)
	if w := membershipRequest(cardsHandler, "POST", "member", `{"kind":"monthly","price_cents":100,"quantity":1}`); w.Code != 403 {
		t.Fatalf("member create = %d", w.Code)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Auth-Username", "admin")
	w := httptest.NewRecorder()
	cardsHandler(w, r)
	if w.Code != 401 {
		t.Fatalf("spoofed admin = %d", w.Code)
	}
	for _, body := range []string{`{"kind":"weekly","price_cents":100,"quantity":1}`, `{"kind":"monthly","price_cents":-1,"quantity":1}`, `{"kind":"yearly","price_cents":1,"quantity":101}`, `{"kind":"monthly","price_cents":1.5,"quantity":1}`} {
		if w := membershipRequest(cardsHandler, "POST", "admin", body); w.Code != 400 {
			t.Fatalf("invalid accepted: %s", body)
		}
	}
	first := generateTestCard(t, "monthly")
	second := generateTestCard(t, "yearly")
	if first.Code == second.Code {
		t.Fatal("duplicate codes")
	}
	var price int
	if err := db.QueryRow("SELECT price_cents FROM plans WHERE kind='monthly'").Scan(&price); err != nil || price != 2990 {
		t.Fatalf("price not saved: %d %v", price, err)
	}
	w = membershipRequest(cardsHandler, "POST", "admin", `{"kind":"monthly","price_cents":0,"quantity":3}`)
	if w.Code != 201 {
		t.Fatal(w.Body)
	}
	if err := db.QueryRow("SELECT price_cents FROM redemption_cards WHERE code=?", first.Code).Scan(&price); err != nil || price != 2990 {
		t.Fatal("existing price changed")
	}
}
func TestRedemptionExtensionAndReplay(t *testing.T) {
	membershipTestDB(t)
	card := generateTestCard(t, "monthly")
	body := fmt.Sprintf(`{"code":%q}`, card.Code)
	before := time.Now().Unix()
	w := membershipRequest(redeemHandler, "POST", "member", body)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	var expiry int64
	db.QueryRow("SELECT expires_at FROM users WHERE username='member'").Scan(&expiry)
	if expiry < before+30*86400 || expiry > time.Now().Unix()+30*86400 {
		t.Fatalf("unexpected expiry %d", expiry)
	}
	for _, user := range []string{"member", "other"} {
		if w = membershipRequest(redeemHandler, "POST", user, body); w.Code != 400 {
			t.Fatalf("replay = %d", w.Code)
		}
	}
	year := generateTestCard(t, "yearly")
	w = membershipRequest(redeemHandler, "POST", "member", fmt.Sprintf(`{"code":%q}`, year.Code))
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	var extended int64
	db.QueryRow("SELECT expires_at FROM users WHERE username='member'").Scan(&extended)
	if extended != expiry+365*86400 {
		t.Fatal("subscription did not extend")
	}
	w = membershipRequest(membershipHandler, "GET", "member", "")
	var data struct {
		History []Card `json:"history"`
	}
	json.Unmarshal(w.Body.Bytes(), &data)
	if len(data.History) != 2 {
		t.Fatal("missing history")
	}
	w = membershipRequest(membershipHandler, "GET", "other", "")
	json.Unmarshal(w.Body.Bytes(), &data)
	if len(data.History) != 0 {
		t.Fatal("history leaked")
	}
}
func TestConcurrentRedemptionOnlyOnce(t *testing.T) {
	membershipTestDB(t)
	card := generateTestCard(t, "monthly")
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for _, user := range []string{"member", "other"} {
		wg.Add(1)
		go func(user string) {
			defer wg.Done()
			results <- membershipRequest(redeemHandler, "POST", user, fmt.Sprintf(`{"code":%q}`, card.Code)).Code
		}(user)
	}
	wg.Wait()
	close(results)
	success := 0
	for code := range results {
		if code == 200 {
			success++
		} else if code != 400 {
			t.Fatalf("unexpected response %d", code)
		}
	}
	if success != 1 {
		t.Fatalf("redeemed %d times", success)
	}
}
func TestExpiryAndFailedRedemptionRollback(t *testing.T) {
	membershipTestDB(t)
	for _, expiry := range []int64{0, time.Now().Add(-time.Hour).Unix()} {
		db.Exec("UPDATE users SET expires_at=? WHERE username='member'", expiry)
		w := membershipRequest(authHandler, "POST", "", `{"auth":"member:Pass123!"}`)
		if w.Code != 403 {
			t.Fatalf("expired auth = %d", w.Code)
		}
	}
	r := httptest.NewRequest("GET", "/subscription/test-subscription", nil)
	w := httptest.NewRecorder()
	serveSubscriptionHandler(w, r)
	if w.Code != 404 {
		t.Fatalf("expired subscription = %d", w.Code)
	}
	card := generateTestCard(t, "monthly")
	db.Exec("UPDATE users SET expires_at=NULL WHERE username='member'")
	w = membershipRequest(redeemHandler, "POST", "member", fmt.Sprintf(`{"code":%q}`, card.Code))
	if w.Code != 400 {
		t.Fatal("unlimited account accepted card")
	}
	w = membershipRequest(redeemHandler, "POST", "other", fmt.Sprintf(`{"code":%q}`, card.Code))
	if w.Code != 200 {
		t.Fatal("failed redemption consumed card", w.Body)
	}
	db.Exec("UPDATE users SET expires_at=?,enabled=0 WHERE username='member'", time.Now().Add(-time.Hour).Unix())
	card = generateTestCard(t, "monthly")
	w = membershipRequest(redeemHandler, "POST", "member", fmt.Sprintf(`{"code":%q}`, card.Code))
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	var enabled bool
	db.QueryRow("SELECT enabled FROM users WHERE username='member'").Scan(&enabled)
	if enabled {
		t.Fatal("redemption bypassed disabled account")
	}
}
func TestLoginSessionAndLogout(t *testing.T) {
	membershipTestDB(t)
	db.Exec("UPDATE admin_users SET password=? WHERE username='member'", hashPassword("Pass123!"))
	w := membershipRequest(loginHandler, "POST", "", `{"username":"member","password":"Pass123!"}`)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("missing protected session")
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(cookies[0])
	if _, err := getRequestUser(r); err != nil {
		t.Fatal(err)
	}
	r.Method = "POST"
	logoutHandler(httptest.NewRecorder(), r)
	if _, err := getRequestUser(r); err == nil {
		t.Fatal("session survived logout")
	}
}

func TestRegistrationRequiresRedemption(t *testing.T) {
	membershipTestDB(t)
	w := membershipRequest(registerHandler, "POST", "", `{"username":"newmember","password":"Preview2026!"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", w.Code, w.Body)
	}
	var expiry int64
	if err := db.QueryRow("SELECT expires_at FROM users WHERE username='newmember'").Scan(&expiry); err != nil || expiry != 0 {
		t.Fatalf("new account must await redemption: %d %v", expiry, err)
	}
	w = membershipRequest(authHandler, "POST", "", `{"auth":"newmember:Preview2026!"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("unredeemed account authenticated: %d", w.Code)
	}
}

func TestMembershipMigrationPreservesExistingUsers(t *testing.T) {
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`CREATE TABLE users(id INTEGER PRIMARY KEY, username TEXT); INSERT INTO users(username) VALUES ('legacy')`); err != nil {
		t.Fatal(err)
	}
	upgradeSchema()
	upgradeSchema()
	if err = initMembership(); err != nil {
		t.Fatal(err)
	}
	if err = initMembership(); err != nil {
		t.Fatal(err)
	}
	var expiry sql.NullInt64
	if err = db.QueryRow("SELECT expires_at FROM users WHERE username='legacy'").Scan(&expiry); err != nil || expiry.Valid {
		t.Fatalf("legacy entitlement changed: %v %v", expiry, err)
	}
}
