package main

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func authMiddleware(token string, next http.Handler) http.Handler {
	tokenBytes := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, `{"error":"missing authorization"}`, http.StatusUnauthorized)
			return
		}
		provided := []byte(strings.TrimPrefix(auth, "Bearer "))
		if subtle.ConstantTimeCompare(provided, tokenBytes) != 1 {
			http.Error(w, `{"error":"invalid token"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
