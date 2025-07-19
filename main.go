package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/yosuke-furukawa/json5/encoding/json5"
)

type Config struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
}

type DiscordUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	Avatar        string `json:"avatar"`
}

var config Config
var firstLogin = map[string]time.Time{}
var sessions = map[string]DiscordUser{}

func main() {
	f, _ := os.Open("config.json5")
	defer f.Close()
	json5.NewDecoder(f).Decode(&config)

	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/callback", callbackHandler)
	http.HandleFunc("/logout", logoutHandler)

	http.ListenAndServe(":8080", nil)
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil || cookie.Value == "" || sessions[cookie.Value].ID == "" {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
			<!DOCTYPE html>
			<html>
			<head><meta charset="UTF-8"><title>Авторизация</title></head>
			<body>
			<h2>Вы не авторизованы</h2>
			<a href="/login"><button>Войти через Discord</button></a>
			</body>
			</html>
		`)
		return
	}

	user := sessions[cookie.Value]
	createdAt := getTimeFromSnowflake(user.ID)
	first := firstLogin[user.ID].Format("02 Jan 2006 15:04")

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
		<!DOCTYPE html>
		<html>
		<head><meta charset="UTF-8"><title>Профиль</title></head>
		<body>
		<h2>Добро пожаловать, %s#%s</h2>
		<img src="https://cdn.discordapp.com/avatars/%s/%s.png" width="128"><br>
		ID: %s<br>
		Аккаунт создан: %s<br>
		Первая авторизация: %s<br><br>
		<a href="/logout"><button>Выйти</button></a>
		</body>
		</html>
	`, user.Username, user.Discriminator, user.ID, user.Avatar, user.ID, createdAt.Format("02 Jan 2006 15:04"), first)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	params := url.Values{}
	params.Add("client_id", config.ClientID)
	params.Add("redirect_uri", config.RedirectURI)
	params.Add("response_type", "code")
	params.Add("scope", "identify guilds")

	http.Redirect(w, r, "https://discord.com/api/oauth2/authorize?"+params.Encode(), http.StatusFound)
}

func callbackHandler(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "code not found", http.StatusBadRequest)
		return
	}

	data := url.Values{}
	data.Set("client_id", config.ClientID)
	data.Set("client_secret", config.ClientSecret)
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", config.RedirectURI)

	req, _ := http.NewRequest("POST", "https://discord.com/api/oauth2/token", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	json.Unmarshal(body, &tokenResp)

	userReq, _ := http.NewRequest("GET", "https://discord.com/api/users/@me", nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	userResp, _ := http.DefaultClient.Do(userReq)
	defer userResp.Body.Close()
	userBody, _ := io.ReadAll(userResp.Body)

	var user DiscordUser
	json.Unmarshal(userBody, &user)

	sessionID := fmt.Sprintf("%s_%d", user.ID, time.Now().UnixNano())
	sessions[sessionID] = user

	if _, exists := firstLogin[user.ID]; !exists {
		firstLogin[user.ID] = time.Now()
	}

	http.SetCookie(w, &http.Cookie{
		Name:  "session",
		Value: sessionID,
		Path:  "/",
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func getTimeFromSnowflake(id string) time.Time {
	snowflakeEpoch := int64(1420070400000)
	idInt := new(bigInt).SetString(id, 10)
	timestamp := idInt.Rsh(idInt, 22).Int64()
	return time.UnixMilli(timestamp + snowflakeEpoch)
}

type bigInt struct{ *big.Int }

func (b *bigInt) SetString(s string, base int) *bigInt {
	i := new(big.Int)
	i.SetString(s, base)
	return &bigInt{i}
}

func (b *bigInt) Rsh(x *bigInt, n uint) *bigInt {
	return &bigInt{new(big.Int).Rsh(x.Int, n)}
}
