package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	syncmap "github.com/immofon/syncmap"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

var LongPollHoldTime = time.Minute * 3
var CheckVersionInteval = time.Second / 2
var DefaultOpListSize = 50

var TLS_KEY = Getenv("TLS_KEY")
var TLS_CERT = Getenv("TLS_CERT")

func Getenv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		fmt.Println("Expect set env ", k)
	}
	return v
}

type SyncMaps struct {
	mu   *sync.Mutex
	maps map[string]*syncmap.SyncMap
}

func NewSyncMaps() *SyncMaps {
	return &SyncMaps{
		mu:   new(sync.Mutex),
		maps: make(map[string]*syncmap.SyncMap),
	}
}

func (sms *SyncMaps) Get(name string) *syncmap.SyncMap {
	sms.mu.Lock()
	defer sms.mu.Unlock()

	return sms.maps[name]
}

func (sms *SyncMaps) Create(name string) *syncmap.SyncMap {
	sms.mu.Lock()
	defer sms.mu.Unlock()

	m := sms.maps[name]
	if m == nil {
		m = syncmap.New(DefaultOpListSize)
		sms.maps[name] = m
		m.Del(name)
		m.ForceAchieve()
	}
	return m
}
func (sms *SyncMaps) GC() {
	sms.mu.Lock()
	mapclone := make(map[string]*syncmap.SyncMap)
	for name, m := range sms.maps {
		mapclone[name] = m
	}
	sms.mu.Unlock()

	for name, m := range mapclone {
		if m.Size() == 0 {
			sms.mu.Lock()
			delete(sms.maps, name)
			sms.mu.Unlock()
		}
	}
}

func main() {
	r := mux.NewRouter()

	syncms := NewSyncMaps()

	go func() {
		for {
			time.Sleep(time.Hour)
			syncms.GC()
		}
	}()

	r.HandleFunc("/sync/map/{name}/{version:[0-9]+}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		name := vars["name"]
		version, err := strconv.ParseInt(vars["version"], 10, 64)
		if err != nil {
			version = 0
		}

		var sm *syncmap.SyncMap
		for i := 0; i < int(LongPollHoldTime/CheckVersionInteval); i++ {
			if sm == nil {
				sm = syncms.Get(name)
			}
			if sm != nil && sm.Version() != version {
				break
			}
			time.Sleep(CheckVersionInteval)
		}

		if sm == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		patch := sm.Diff(version)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(patch)
	}).Methods(http.MethodGet)

	r.HandleFunc("/sync/map/{name}/{key}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		name := vars["name"]
		key := vars["key"]
		priority, _ := strconv.Atoi(vars["priority"])

		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		syncms.Create(name).SetWithPriority(key, string(data), priority)
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodPost).Queries("priority", "{priority:[0-9]*}")

	r.HandleFunc("/sync/map/{name}/{key}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		name := vars["name"]
		key := vars["key"]

		sm := syncms.Get(name)
		if sm != nil {
			sm.Del(key)
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}).Methods(http.MethodDelete)

	r.Use(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			h.ServeHTTP(w, r)
		})
	})

	hdr := handlers.LoggingHandler(os.Stdout, r)
	hdr = handlers.CompressHandler(hdr)

	err := http.ListenAndServeTLS(":3669", TLS_CERT, TLS_KEY, hdr)
	if err != nil {
		panic(err)
	}
}
