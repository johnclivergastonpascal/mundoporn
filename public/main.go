package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"sync"
)

// =====================
// 🔥 STRUCTS
// =====================

type Video struct {
	ID     string
	Title  string
	Image  string
	Source string
}

type EpornerResponse struct {
	Videos []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Thumb string `json:"default_thumb"`
	} `json:"videos"`
}

type PHResponse struct {
	Videos []struct {
		VideoID string `json:"video_id"`
		Title   string `json:"title"`
		Thumb   string `json:"thumb"`
	} `json:"videos"`
}

// =====================
// 🔥 SITEMAP
// =====================

type Image struct {
	Loc   string `xml:"image:loc"`
	Title string `xml:"image:title"`
}

type SitemapURL struct {
	Loc        string  `xml:"loc"`
	Changefreq string  `xml:"changefreq"`
	Priority   string  `xml:"priority"`
	Images     []Image `xml:"image:image,omitempty"`
}

type URLSet struct {
	Xmlns   string       `xml:"xmlns,attr"`
	ImageNS string       `xml:"xmlns:image,attr"`
	URLs    []SitemapURL `xml:"url"`
}

// =====================
// 🔥 GLOBAL FETCH
// =====================

func fetchEporner(page int) []Video {
	api := fmt.Sprintf("https://www.eporner.com/api/v2/video/search/?query=latest&per_page=30&page=%d", page)

	resp, err := http.Get(api)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)

	var data EpornerResponse
	json.Unmarshal(b, &data)

	var out []Video
	for _, v := range data.Videos {
		out = append(out, Video{
			ID:     v.ID,
			Title:  v.Title,
			Image:  v.Thumb,
			Source: "eporner",
		})
	}
	return out
}

func fetchPornhub(page int) []Video {
	api := fmt.Sprintf("https://www.pornhub.com/webmasters/search?page=%d", page)

	resp, err := http.Get(api)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)

	var data PHResponse
	json.Unmarshal(b, &data)

	var out []Video
	for _, v := range data.Videos {
		out = append(out, Video{
			ID:     v.VideoID,
			Title:  v.Title,
			Image:  v.Thumb,
			Source: "pornhub",
		})
	}
	return out
}

// =====================
// 🔥 SAVE
// =====================

func save(file string, data URLSet) {
	f, _ := os.Create(file)
	defer f.Close()

	f.Write([]byte(xml.Header))

	enc := xml.NewEncoder(f)
	enc.Indent("", "  ")
	enc.Encode(data)
}

// =====================
// 🔥 GENERATOR FIX FINAL (NO FREEZE)
// =====================

func GenerateSitemap(domain string) {
	const maxURLs = 55555
	const workers = 5

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sitemap := URLSet{
		Xmlns:   "http://www.sitemaps.org/schemas/sitemap/0.9",
		ImageNS: "http://www.google.com/schemas/sitemap-image/1.1",
	}

	seen := sync.Map{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)

	fmt.Println("🚀 Running (ANTI FREEZE VERSION)...")

	for page := 1; page <= 2000; page++ {

		if len(sitemap.URLs) >= maxURLs {
			cancel()
			break
		}

		wg.Add(1)

		go func(p int) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			videos := append(fetchEporner(p), fetchPornhub(p)...)

			for _, v := range videos {

				if len(sitemap.URLs) >= maxURLs {
					cancel()
					return
				}

				title := neturl.QueryEscape(v.Title)
				loc := fmt.Sprintf("%s/video/%s/%s", domain, v.ID, title)

				if _, loaded := seen.LoadOrStore(loc, true); loaded {
					continue
				}

				mu.Lock()

				if len(sitemap.URLs) < maxURLs {
					sitemap.URLs = append(sitemap.URLs, SitemapURL{
						Loc:        loc,
						Changefreq: "daily",
						Priority:   "0.8",
						Images: []Image{
							{Loc: v.Image, Title: v.Title},
						},
					})
				}

				mu.Unlock()
			}

		}(page)
	}

	wg.Wait()

	save("sitemap.xml", sitemap)

	fmt.Println("\n✅ DONE:", len(sitemap.URLs))
}

func main() {
	GenerateSitemap("https://mundoporn.onrender.com")
}
