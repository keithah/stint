package tzcache

import (
	"strings"
	"sync"
	"time"
)

var locations sync.Map

func Location(name string) *time.Location {
	name = strings.TrimSpace(name)
	if name == "" {
		return time.UTC
	}
	if cached, ok := locations.Load(name); ok {
		return cached.(*time.Location)
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	actual, _ := locations.LoadOrStore(name, location)
	return actual.(*time.Location)
}
