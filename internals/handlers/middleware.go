package handlers

import (
	"net/http"
	"time"
)

func SetDate(next http.Handler) http.Handler {
	dateStr := time.Now().Format(time.RFC1123)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("date", dateStr)
		next.ServeHTTP(w, r)
	})
}

func SetContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("content-type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}
