// Package mitv talks to the LAN remote API of Xiaomi HyperOS displays and
// TVs: the HTTP server on port 6095 that switches inputs and changes volume.
package mitv

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Port is where the remote API listens.
const Port = 6095

// ErrRejected means the device answered but refused the request.
var ErrRejected = errors.New("mitv: request rejected")

// Source is an input of the display, such as HDMI1 (23) or DP (29).
type Source struct {
	ID   int
	Name string
}

// Link is a client for one device.
type Link struct {
	Host   string
	client *http.Client
}

func New(host string) *Link {
	return &Link{Host: host, client: &http.Client{Timeout: 4 * time.Second}}
}

// DeviceName answers whether the device is reachable, with its name.
func (l *Link) DeviceName() (string, error) {
	var data struct {
		DeviceName string `json:"devicename"`
	}
	err := l.call("request", url.Values{"action": {"isalive"}}, &data)
	return data.DeviceName, err
}

func (l *Link) Sources() ([]Source, error) {
	var data struct {
		SourceList []struct {
			SourceID   int    `json:"sourceId"`
			SourceName string `json:"sourceName"`
		} `json:"sourceList"`
	}
	if err := l.call("controller", url.Values{"action": {"getsources"}}, &data); err != nil {
		return nil, err
	}
	sources := make([]Source, len(data.SourceList))
	for i, s := range data.SourceList {
		sources[i] = Source{ID: s.SourceID, Name: s.SourceName}
	}
	return sources, nil
}

// Select switches input. changesource2 takes the numeric id, so it also
// reaches DP and USB-C, which the name-based changesource rejects.
func (l *Link) Select(sourceID int) error {
	return l.call("controller", url.Values{"action": {"changesource2"}, "sourceid": {strconv.Itoa(sourceID)}}, nil)
}

// Volume reads the volume on a 0...100 scale.
func (l *Link) Volume() (int, error) {
	var data struct {
		Volume    int `json:"volume"`
		MaxVolume int `json:"maxVolume"`
	}
	if err := l.call("controller", url.Values{"action": {"getvolume"}}, &data); err != nil {
		return 0, err
	}
	return data.Volume * 100 / max(data.MaxVolume, 1), nil
}

// Press sends a remote key such as "volumeup", "volumedown", "home" or "back".
func (l *Link) Press(key string) error {
	return l.call("controller", url.Values{"action": {"keyevent"}, "keycode": {key}}, nil)
}

func (l *Link) call(path string, query url.Values, data any) error {
	u := url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", l.Host, Port), Path: "/" + path, RawQuery: query.Encode()}
	resp, err := l.client.Get(u.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var envelope struct {
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if envelope.Msg != "success" {
		return ErrRejected
	}
	if data == nil || len(envelope.Data) == 0 {
		return nil
	}
	return json.Unmarshal(envelope.Data, data)
}
