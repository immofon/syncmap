package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"tmp/syncmap"

	"github.com/gorilla/mux"
)

func main() {
	r := mux.NewRouter()

	sm := syncmap.New(100)
	r.HandleFunc("/syncmap/{name}/{version:[0-9]+}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		name := vars["name"]
		version, err := strconv.ParseInt(vars["version"], 10, 64)
		if err != nil {
			version = 0
		}
		fmt.Println(name)

		if name == "test" {
			patch := sm.Diff(version)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(patch)
		} else {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}).Methods(http.MethodGet)

	r.HandleFunc("/syncmap/{name}/{key}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		name := vars["name"]
		key := vars["key"]

		if name == "test" {
			data, err := ioutil.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			sm.Set(key, string(data))
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
			return
		}

	}).Methods(http.MethodPost)
	r.HandleFunc("/syncmap/{name}/{key}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		name := vars["name"]
		key := vars["key"]

		if name == "test" {
			sm.Del(key)
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}).Methods(http.MethodDelete)

	r.Use(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Println(r.URL)
			w.Header().Set("Access-Control-Allow-Origin", "*")
			h.ServeHTTP(w, r)
		})
	})

	http.ListenAndServe(":9899", r)
}
