package main

import (
	"net/http"
	"strings"

	"cloud.google.com/go/storage"
)

func getHandler(c Config, client *storage.Client) http.HandlerFunc {
	mapHandler := getMapHandler(c, client)

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, c.MapPrefix):
			r.URL.Path = strings.Replace(r.URL.Path, c.MapPrefix, "", 1)
			mapHandler(w, r)
		case r.URL.Path == "/":
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}
}
