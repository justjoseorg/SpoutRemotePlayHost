// Package artwork finds cover art for a game name on SteamGridDB.
package artwork

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Kinds match the asset types of Steam's SetCustomArtworkForApp.
const (
	Portrait = 0
	Hero     = 1
	Logo     = 2
	Wide     = 3
	Icon     = 4
)

const maxImage = 8 << 20

// Image is one downloaded piece of artwork.
type Image struct {
	Kind int
	Ext  string
	Data []byte
}

// Client talks to the SteamGridDB API; the API key is kept in a private file.
type Client struct {
	Base string
	path string
	mu   sync.Mutex
	key  string
	http *http.Client
}

// Open loads the saved key (if any) from path.
func Open(path string) *Client {
	c := &Client{Base: "https://www.steamgriddb.com/api/v2", path: path, http: &http.Client{Timeout: 20 * time.Second}}
	if b, err := os.ReadFile(path); err == nil {
		c.key = strings.TrimSpace(string(b))
	}
	return c
}

func (c *Client) HasKey() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.key != ""
}

// SetKey stores the API key; an empty key removes it.
func (c *Client) SetKey(k string) error {
	k = strings.TrimSpace(k)
	c.mu.Lock()
	defer c.mu.Unlock()
	if k == "" {
		c.key = ""
		if err := os.Remove(c.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.WriteFile(c.path, []byte(k), 0o600); err != nil {
		return err
	}
	c.key = k
	return nil
}

func (c *Client) get(rawURL string, auth bool, out any) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	if auth {
		c.mu.Lock()
		req.Header.Set("Authorization", "Bearer "+c.key)
		c.mu.Unlock()
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return errors.New("SteamGridDB rejected the API key")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("SteamGridDB replied %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxImage)).Decode(out)
}

func (c *Client) download(u string) ([]byte, error) {
	resp, err := c.http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download replied %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxImage+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxImage {
		return nil, errors.New("image too large")
	}
	return b, nil
}

type listing struct {
	Data []struct {
		ID   int    `json:"id"`
		URL  string `json:"url"`
		Mime string `json:"mime"`
	} `json:"data"`
}

// Find searches for name and downloads the best match's portrait, hero, logo and icon.
// Kinds with no artwork are skipped.
func (c *Client) Find(name string) ([]Image, error) {
	if !c.HasKey() {
		return nil, errors.New("no SteamGridDB API key set")
	}
	var found listing
	if err := c.get(c.Base+"/search/autocomplete/"+url.PathEscape(name), true, &found); err != nil {
		return nil, err
	}
	if len(found.Data) == 0 {
		return nil, fmt.Errorf("no SteamGridDB match for %q", name)
	}
	game := found.Data[0].ID
	kinds := []struct {
		kind  int
		path  string
		query string
	}{
		{Portrait, "grids", "?dimensions=600x900&types=static"},
		{Wide, "grids", "?dimensions=920x430,460x215&types=static"},
		{Hero, "heroes", "?types=static"},
		{Logo, "logos", "?types=static"},
		{Icon, "icons", "?types=static"},
	}
	var out []Image
	for _, k := range kinds {
		var l listing
		if err := c.get(fmt.Sprintf("%s/%s/game/%d%s", c.Base, k.path, game, k.query), true, &l); err != nil || len(l.Data) == 0 {
			continue
		}
		ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}[l.Data[0].Mime]
		if ext == "" {
			continue
		}
		b, err := c.download(l.Data[0].URL)
		if err != nil {
			continue
		}
		out = append(out, Image{Kind: k.kind, Ext: ext, Data: b})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("SteamGridDB has no artwork for %q", name)
	}
	return out, nil
}
