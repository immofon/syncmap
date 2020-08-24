package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"
	"tmp/syncmap"

	"github.com/gorilla/mux"
)

var LongPollHoldTime = time.Minute * 3
var CheckVersionInteval = time.Second / 2

func main() {
	r := mux.NewRouter()

	sm := syncmap.New(100)
	expectname := os.Getenv("name")
	if expectname == "" {
		expectname = "test"
	}

	r.HandleFunc("/sync/map/{name}/{version:[0-9]+}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		name := vars["name"]
		version, err := strconv.ParseInt(vars["version"], 10, 64)
		if err != nil {
			version = 0
		}
		fmt.Println(name)

		if name == expectname {
			for i := 0; i < int(LongPollHoldTime/CheckVersionInteval); i++ {
				if sm.Version() != version {
					break
				}
				time.Sleep(CheckVersionInteval)
			}

			patch := sm.Diff(version)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(patch)
		} else {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}).Methods(http.MethodGet)

	r.HandleFunc("/sync/map/{name}/{key}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		name := vars["name"]
		key := vars["key"]

		if name == expectname {
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

	r.HandleFunc("/sync/map/{name}/{key}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		name := vars["name"]
		key := vars["key"]

		if name == expectname {
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

	http.ListenAndServe(":3669", r)
}
