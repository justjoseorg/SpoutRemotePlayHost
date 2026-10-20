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
	"path"
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
		c.key = cleanKey(string(b))
	}
	return c
}

// cleanKey drops a label pasted along with the key (e.g. "Key: 0123…"): keys contain no spaces.
func cleanKey(k string) string {
	f := strings.Fields(k)
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}

func (c *Client) HasKey() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.key != ""
}

// SetKey stores the API key; an empty key removes it.
func (c *Client) SetKey(k string) error {
	k = cleanKey(k)
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
		ID    int    `json:"id"`
		Name  string `json:"name"`
		URL   string `json:"url"`
		Thumb string `json:"thumb"`
		Mime  string `json:"mime"`
	} `json:"data"`
}

// Game is a SteamGridDB search match.
type Game struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Candidate is one image SteamGridDB offers for a kind of artwork.
type Candidate struct {
	URL   string `json:"url"`
	Thumb string `json:"thumb"`
	Mime  string `json:"mime"`
}

// Choices lists the artwork SteamGridDB has for one game, best first.
type Choices struct {
	Games []Game              `json:"games"`
	Game  int                 `json:"game"`
	Kinds map[int][]Candidate `json:"kinds"`
}

var kindQueries = []struct {
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

var mimeExt = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}

// maxChoices caps how many images per kind are offered.
const maxChoices = 24

// Options searches for name and lists the artwork of game (or of the best match when game is 0).
func (c *Client) Options(name string, game int) (Choices, error) {
	if !c.HasKey() {
		return Choices{}, errors.New("no SteamGridDB API key set")
	}
	var found listing
	if err := c.get(c.Base+"/search/autocomplete/"+url.PathEscape(name), true, &found); err != nil {
		return Choices{}, err
	}
	ch := Choices{Kinds: map[int][]Candidate{}}
	for _, g := range found.Data {
		ch.Games = append(ch.Games, Game{ID: g.ID, Name: g.Name})
	}
	if game == 0 {
		if len(found.Data) == 0 {
			return Choices{}, fmt.Errorf("no SteamGridDB match for %q", name)
		}
		game = found.Data[0].ID
	}
	ch.Game = game
	for _, k := range kindQueries {
		var l listing
		if err := c.get(fmt.Sprintf("%s/%s/game/%d%s", c.Base, k.path, game, k.query), true, &l); err != nil {
			if strings.Contains(err.Error(), "API key") {
				return Choices{}, err
			}
			continue
		}
		for _, d := range l.Data {
			if mimeExt[d.Mime] == "" || !c.allowed(d.URL) {
				continue
			}
			thumb := d.Thumb
			if !c.allowed(thumb) {
				thumb = d.URL
			}
			ch.Kinds[k.kind] = append(ch.Kinds[k.kind], Candidate{URL: d.URL, Thumb: thumb, Mime: d.Mime})
			if len(ch.Kinds[k.kind]) == maxChoices {
				break
			}
		}
	}
	return ch, nil
}

// allowed limits downloads to SteamGridDB (and its CDN), never arbitrary URLs.
func (c *Client) allowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}
	host := u.Hostname()
	if b, err := url.Parse(c.Base); err == nil && host == b.Hostname() {
		return true
	}
	return u.Scheme == "https" && (host == "steamgriddb.com" || strings.HasSuffix(host, ".steamgriddb.com"))
}

// Fetch downloads one chosen image.
func (c *Client) Fetch(kind int, raw string) (Image, error) {
	if kind < Portrait || kind > Icon {
		return Image{}, errors.New("unknown artwork kind")
	}
	if !c.allowed(raw) {
		return Image{}, errors.New("artwork must come from SteamGridDB")
	}
	ext := ""
	switch strings.ToLower(path.Ext(strings.SplitN(raw, "?", 2)[0])) {
	case ".png":
		ext = "png"
	case ".jpg", ".jpeg":
		ext = "jpg"
	case ".webp":
		ext = "webp"
	default:
		return Image{}, errors.New("unsupported image type")
	}
	b, err := c.download(raw)
	if err != nil {
		return Image{}, err
	}
	return Image{Kind: kind, Ext: ext, Data: b}, nil
}

// Find downloads the top choice of each kind for the best match of name.
// Kinds with no artwork are skipped.
func (c *Client) Find(name string) ([]Image, error) {
	ch, err := c.Options(name, 0)
	if err != nil {
		return nil, err
	}
	var out []Image
	for _, k := range kindQueries {
		for _, cand := range ch.Kinds[k.kind] {
			b, err := c.download(cand.URL)
			if err != nil {
				continue
			}
			out = append(out, Image{Kind: k.kind, Ext: mimeExt[cand.Mime], Data: b})
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("SteamGridDB has no artwork for %q", name)
	}
	return out, nil
}
