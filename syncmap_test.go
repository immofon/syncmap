package syncmap

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestSyncMap(t *testing.T) {
	sm := New(2)
	sm.Set("hello", "syncmap")
	sm.Set("name", "mofon")
	sm.Set("email", "immofon@163.com")
	sm.Set("want to delete", "nonsense")
	backup := sm.Diff(-1)
	sm.Del("want to delete")

	if sm.Get("hello").V != "syncmap" {
		t.Fatal()
	}
	if sm.Get("name").V != "mofon" {
		t.Fatal()
	}
	if sm.Get("want to delete").V != "" {
		t.Fatal()
	}
	sm.Patch(backup)
	if sm.Get("want to delete").V != "nonsense" {
		t.Fatal()
	}

	msg := sm.Diff(0)
	data, _ := json.MarshalIndent(msg, "", "  ")
	fmt.Println(string(data))

}
