package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	releasesAPI  = "https://api.github.com/repos/jaba0x/HassTray/releases/latest"
	releasesPage = "https://github.com/jaba0x/HassTray/releases/latest"
)

// UpdateInfo says whether GitHub has a newer release than the one running.
type UpdateInfo struct {
	Available bool   `json:"available"`
	Latest    string `json:"latest"`
	URL       string `json:"url"`
}

var (
	updMu   sync.Mutex
	updLast UpdateInfo
)

func lastUpdate() UpdateInfo {
	updMu.Lock()
	defer updMu.Unlock()
	return updLast
}

// watchUpdates asks GitHub once a day. Failures are ignored: the notice is a
// nicety and should never get in the way.
func watchUpdates(onChange func()) {
	for {
		if info, ok := checkUpdate(); ok {
			updMu.Lock()
			updLast = info
			updMu.Unlock()
			onChange()
		}
		time.Sleep(24 * time.Hour)
	}
}

func checkUpdate() (UpdateInfo, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, releasesAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "HassTray/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return UpdateInfo{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return UpdateInfo{}, false
	}
	var rel struct {
		Tag string `json:"tag_name"`
		URL string `json:"html_url"`
	}
	if json.NewDecoder(resp.Body).Decode(&rel) != nil || rel.Tag == "" {
		return UpdateInfo{}, false
	}
	latest := strings.TrimPrefix(rel.Tag, "v")
	return UpdateInfo{Available: newer(latest, Version), Latest: latest, URL: rel.URL}, true
}

// newer reports whether version a is higher than b, comparing dotted numbers.
func newer(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(strings.SplitN(as[i], "-", 2)[0])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(strings.SplitN(bs[i], "-", 2)[0])
		}
		if x != y {
			return x > y
		}
	}
	return false
}
