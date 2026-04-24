package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
)

// --- ESTRUCTURAS ---

type Video struct {
	ID     string `xml:"-" json:"id"`
	Title  string `xml:"-" json:"title"`
	Source string `xml:"-" json:"source"`
}

type PHVideo struct {
	VideoID string `json:"video_id"`
	Title   string `json:"title"`
}

type PHResponse struct {
	Videos []PHVideo `json:"videos"`
}

type APIResponse struct {
	Videos []Video `json:"videos"`
}

type URL struct {
	Loc        string `xml:"loc"`
	Changefreq string `xml:"changefreq"`
	Priority   string `xml:"priority"`
}

type URLSet struct {
	XMLName xml.Name `xml:"urlset"`
	Xmlns   string   `xml:"xmlns,attr"`
	URLs    []URL    `xml:"url"`
}

// --- FETCHERS ---

func fetchEporner(page int) []Video {
	url := fmt.Sprintf("https://www.eporner.com/api/v2/video/search/?query=latest&per_page=30&page=%d", page)
	resp, err := http.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var result APIResponse
	json.NewDecoder(resp.Body).Decode(&result)
	for i := range result.Videos {
		result.Videos[i].Source = "eporner"
	}
	return result.Videos
}

func fetchPornhub(page int) []Video {
	url := fmt.Sprintf("https://www.pornhub.com/webmasters/search?page=%d", page)
	resp, err := http.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var result PHResponse
	json.NewDecoder(resp.Body).Decode(&result)

	var normalized []Video
	for _, v := range result.Videos {
		normalized = append(normalized, Video{ID: v.VideoID, Title: v.Title, Source: "pornhub"})
	}
	return normalized
}

// --- GENERADOR ---

func GenerateSitemap(domain string) {
	const maxURLs = 50000 // Cambiado a 50k para tu objetivo real
	const concurrentLimit = 15

	sitemap := URLSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
	}

	// Mapa para evitar duplicados y Mutex para protegerlo en concurrencia
	seenURLs := make(map[string]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrentLimit)

	// Añadir Home
	home := domain + "/"
	sitemap.URLs = append(sitemap.URLs, URL{Loc: home, Changefreq: "daily", Priority: "1.0"})
	seenURLs[home] = true

	pagesNeeded := (maxURLs / 50) + 2

	fmt.Println("🚀 Iniciando recolección de URLs únicas...")

	for p := 1; p <= pagesNeeded; p++ {
		mu.Lock()
		if len(sitemap.URLs) >= maxURLs {
			mu.Unlock()
			break
		}
		mu.Unlock()

		wg.Add(1)
		go func(page int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ep := fetchEporner(page)
			ph := fetchPornhub(page)
			combined := append(ep, ph...)

			mu.Lock()
			defer mu.Unlock()

			for _, v := range combined {
				if len(sitemap.URLs) >= maxURLs {
					return
				}

				// Limpiar título
				cleanTitle := strings.Map(func(r rune) rune {
					if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
						return r
					}
					return '+'
				}, v.Title)

				// Crear la URL final
				loc := fmt.Sprintf("%s/video/?id=%s&title=%s&source=%s", domain, v.ID, cleanTitle, v.Source)

				// VERIFICACIÓN DE DUPLICADOS
				if !seenURLs[loc] {
					seenURLs[loc] = true
					sitemap.URLs = append(sitemap.URLs, URL{
						Loc:        loc,
						Changefreq: "weekly",
						Priority:   "0.6",
					})
				}
			}
			fmt.Printf("\rProcesadas: %d únicas", len(sitemap.URLs))
		}(p)
	}

	wg.Wait()

	filePath := "sitemap.xml"
	file, err := os.Create(filePath)
	if err != nil {
		fmt.Printf("\n❌ Error: %v\n", err)
		return
	}
	defer file.Close()

	file.Write([]byte(xml.Header))
	enc := xml.NewEncoder(file)
	enc.Indent("", "  ")
	enc.Encode(sitemap)

	fmt.Printf("\n✅ Sitemap finalizado con %d URLs en %s\n", len(sitemap.URLs), filePath)
}

func main() {
	GenerateSitemap("http://localhost:8080")
}
