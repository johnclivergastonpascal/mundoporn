package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
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

// --- FUNCIÓN DE GUARDADO FÍSICO ---
func saveToDisk(filePath string, data URLSet) {
	if _, err := os.Stat("public"); os.IsNotExist(err) {
		os.Mkdir("public", 0755)
	}
	file, err := os.Create(filePath)
	if err != nil {
		fmt.Printf("\n❌ Error al guardar: %v\n", err)
		return
	}
	defer file.Close()
	file.Write([]byte(xml.Header))
	enc := xml.NewEncoder(file)
	enc.Indent("", "  ")
	enc.Encode(data)
}

// --- GENERADOR INCREMENTAL SEGURO ---
func GenerateSitemap(domain string) {
	const maxURLs = 50000
	const concurrentLimit = 5 // Reducido para no ser bloqueado por las APIs
	filePath := "public/sitemap.xml"

	sitemap := URLSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
	}
	seenURLs := make(map[string]bool)

	// 1. Cargar datos previos
	existingFile, err := os.Open(filePath)
	if err == nil {
		byteValue, _ := io.ReadAll(existingFile)
		xml.Unmarshal(byteValue, &sitemap)
		existingFile.Close()
		for _, u := range sitemap.URLs {
			seenURLs[u.Loc] = true
		}
		fmt.Printf("📂 Recuperadas %d URLs del archivo.\n", len(sitemap.URLs))
	} else {
		home := domain + "/"
		sitemap.URLs = append(sitemap.URLs, URL{Loc: home, Changefreq: "daily", Priority: "1.0"})
		seenURLs[home] = true
	}

	// 2. Control de recolección
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrentLimit)

	newURLsCounter := 0 // Contador para el autoguardado cada 1000

	fmt.Println("🚀 Iniciando recorrido exhaustivo hasta 50k...")

	// Recorremos hasta 2000 páginas para asegurar los 50k
	for p := 1; p <= 2000; p++ {
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

			addedInThisBatch := 0
			for _, v := range combined {
				if len(sitemap.URLs) >= maxURLs {
					return
				}

				cleanTitle := strings.Map(func(r rune) rune {
					if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
						return r
					}
					return '+'
				}, v.Title)

				loc := fmt.Sprintf("%s/video/?id=%s&title=%s&source=%s", domain, v.ID, cleanTitle, v.Source)

				if !seenURLs[loc] {
					seenURLs[loc] = true
					sitemap.URLs = append(sitemap.URLs, URL{Loc: loc, Changefreq: "weekly", Priority: "0.6"})
					newURLsCounter++
					addedInThisBatch++
				}
			}

			// --- LÓGICA DE AUTOGUARDADO CADA 1000 ---
			if newURLsCounter >= 1000 {
				fmt.Printf("\n💾 Backup automático: %d URLs alcanzadas. Guardando...", len(sitemap.URLs))
				saveToDisk(filePath, sitemap)
				newURLsCounter = 0
			}

			fmt.Printf("\rProgreso: %d / %d URLs", len(sitemap.URLs), maxURLs)
		}(p)
	}

	wg.Wait()

	// 3. Guardado final
	saveToDisk(filePath, sitemap)
	fmt.Printf("\n✅ Proceso finalizado. Sitemap listo con %d URLs.\n", len(sitemap.URLs))
}

func main() {
	GenerateSitemap("https://mundoporn.onrender.com")
}
