package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"sync"
	"time"
	"golang.org/x/crypto/bcrypt"
)

var loginMessage = ""

var loginTemplate = template.Must(template.ParseFiles("templates/login.html"))

type loginPageData struct {
	Message string `json:"message"`
}

type session struct {
	username string
	userIP   string
	expires  time.Time
}

var sessions = map[string]session{} // token -> session
var sessionMutex sync.Mutex // mutex for sessions list


type ctxKey struct{}

func logInfo(msg string) {
	fmt.Println("[" + time.Now().Format("2006-01-02 15:04:05") + "] " + msg)
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	username, password, userIP := r.PostFormValue("username"), r.PostFormValue("password"), r.Header.Get("X-Real-IP")

	// Check if username matches
	username_ok := true
	if username != os.Getenv("ADMIN_USERNAME") {
		username_ok = false
	}


	// if the user is already signed in somewhere else, invalidate the old session
	for token, s := range sessions {
		if username == s.username {
			logInfo("Invalidate old session for user: " + username + ", IP: " + s.userIP)
			sessionMutex.Lock()
			delete(sessions, token)
			sessionMutex.Unlock()
		}
	}

	// Check if password matches
	err := bcrypt.CompareHashAndPassword([]byte(os.Getenv("ADMIN_PASSWORD_HASH")), []byte(password))

	// If not, throw error
	if !username_ok || err != nil {
		logInfo("Failed login by user: " + username + ", IP: " + userIP)
		loginMessage = "Invalid credentials"
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Else create new token
	token, err := newToken()
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	exp := time.Now().Add(1 * time.Hour)
	sessionMutex.Lock()
	sessions[token] = session{username, userIP, exp}
	sessionMutex.Unlock()

	// Set the new cookie in the broswer
	http.SetCookie(w, &http.Cookie{
		Name:     "simple_kvm_token",
		Value:    token,
		Path:     "/",
		Expires:  exp,
		HttpOnly: true,
		Secure:   true, // requires HTTPS (except localhost in most browsers)
		SameSite: http.SameSiteLaxMode,
	})

	logInfo("Login by user: " + username + ", IP: " + userIP)
	loginMessage = ""
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func loginPage(w http.ResponseWriter, r *http.Request) {
	if err := loginTemplate.Execute(w, loginPageData{Message: loginMessage}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("simple_kvm_token"); err == nil {
		sessionMutex.Lock()
		delete(sessions, c.Value)
		sessionMutex.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "simple_kvm_token", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if token exists
		c, err := r.Cookie("simple_kvm_token")
		if err != nil {
			// If not, redirect to /login
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Check if token is correct
		sessionMutex.Lock()
		s, ok := sessions[c.Value]
		if ok && time.Now().After(s.expires) {
			delete(sessions, c.Value)
			ok = false
		}
		sessionMutex.Unlock()

		// If not, redirect to /login
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Context that is cancelled when the session dies or the handler returns
		ctx, cancel := context.WithCancel(context.WithValue(r.Context(), ctxKey{}, s.username))
		defer cancel()

		// Check if session is still valid every second
		go func() {
			t := time.NewTicker(time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if !sessionValid(c.Value) {
						cancel()
						return
					}
				}
			}
		}()

		next(w, r.WithContext(ctx))
	}
}

func sessionValid(token string) bool {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	s, ok := sessions[token]
	return ok && time.Now().Before(s.expires)
}
