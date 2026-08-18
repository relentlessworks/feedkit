package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func wantsJSON(r *http.Request) bool {
	if r.URL.Query().Get("format") == "json" {
		return true
	}
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

func writeRecord(w http.ResponseWriter, r *http.Request, fields []string) {
	if wantsJSON(r) {
		m := make(map[string]string)
		for _, f := range fields {
			parts := strings.SplitN(f, "=", 2)
			if len(parts) == 2 {
				m[parts[0]] = parts[1]
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintln(w, strings.Join(fields, " "))
}

func writeRecords(w http.ResponseWriter, r *http.Request, records [][]string) {
	if wantsJSON(r) {
		var list []map[string]string
		for _, fields := range records {
			m := make(map[string]string)
			for _, f := range fields {
				parts := strings.SplitN(f, "=", 2)
				if len(parts) == 2 {
					m[parts[0]] = parts[1]
				}
			}
			list = append(list, m)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	for _, fields := range records {
		fmt.Fprintln(w, strings.Join(fields, " "))
	}
}

func writeError(w http.ResponseWriter, r *http.Request, status int, msg, hint string) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{
			"error": msg,
			"hint":  hint,
		})
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	fmt.Fprintf(w, "error: %s | hint: %s\n", msg, hint)
}

func writeOK(w http.ResponseWriter, r *http.Request, msg string) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"ok": msg})
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintln(w, "ok: "+msg)
}
