package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// --- ESTRUCTURAS ---

type ThumbInfo struct {
	Src string `json:"src"`
}

type Video struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Keywords  string    `json:"keywords"`
	Views     int       `json:"views"`
	Rate      string    `json:"rate"`
	URL       string    `json:"url"`
	LengthMin string    `json:"length_min"`
	Thumb     ThumbInfo `json:"default_thumb"`
	Source    string    `json:"source"`
}

type PHCategory struct {
	Category string `json:"category"`
}
type PHTag struct {
	TagName string `json:"tag_name"`
}
type PHPornstar struct {
	PornstarName string `json:"pornstar_name"`
}

type PHVideo struct {
	Title        string       `json:"title"`
	VideoID      string       `json:"video_id"`
	DefaultThumb string       `json:"default_thumb"`
	Duration     string       `json:"duration"`
	Rating       float64      `json:"rating"`
	Views        int          `json:"views"`
	Tags         []PHTag      `json:"tags"`
	Categories   []PHCategory `json:"categories"`
	Pornstars    []PHPornstar `json:"pornstars"`
}

type PHResponse struct {
	Videos []PHVideo `json:"videos"`
}
type APIResponse struct {
	Videos []Video `json:"videos"`
}

// IndexData - Estructura para la página principal con datos SEO
type IndexData struct {
	Videos        []Video
	Categories    []string
	CurrentPage   int
	NextPage      int
	PrevPage      int
	Query         string
	CurrentURL    string
	CurrentTime   string
	IsVideo       bool
	VideoEmbedURL string
}

// DetailsData - Estructura para la página de detalles con datos SEO
type DetailsData struct {
	MainVideo     Video
	Related       []Video
	Categories    []string
	Query         string
	CurrentURL    string
	CurrentTime   string
	IsVideo       bool
	VideoEmbedURL string
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

// --- HELPERS PARA TEMPLATES ---

// add - Suma dos enteros
func add(a, b int) int {
	return a + b
}

// sub - Resta dos enteros
func sub(a, b int) int {
	return a - b
}

// urlEncode - Codifica una cadena para URL
func urlEncode(s string) string {
	return strings.ReplaceAll(url.PathEscape(s), "%20", "+")
}

// lastSeparator - Devuelve "," si no es el último elemento
func lastSeparator(i int, total int) string {
	if i < total-1 {
		return ","
	}
	return ""
}

// --- LÓGICA DE EXTRACCIÓN DINÁMICA ---

// extractDynamicMenu toma los videos de PH y saca categorías, tags y pornstars únicos
func extractDynamicMenu(phVideos []PHVideo) []string {
	uniqueMap := make(map[string]bool)
	var result []string

	for _, v := range phVideos {
		// Sacar Pornstars
		for _, p := range v.Pornstars {
			uniqueMap[p.PornstarName] = true
		}
		// Sacar Categorías
		for _, c := range v.Categories {
			uniqueMap[c.Category] = true
		}
		// Sacar Tags (limitado para no saturar)
		for i, t := range v.Tags {
			if i > 2 {
				break
			} // Solo 2 tags por video para variedad
			uniqueMap[t.TagName] = true
		}
	}

	for key := range uniqueMap {
		if key != "" {
			result = append(result, key)
		}
	}

	// Si no hay resultados, devolver fallback básico
	if len(result) == 0 {
		return []string{"Amateur", "Latina", "Teen", "Milf"}
	}

	// Mezclar y limitar a 15 para el menú
	rand.Shuffle(len(result), func(i, j int) { result[i], result[j] = result[j], result[i] })
	if len(result) > 15 {
		result = result[:15]
	}
	return result
}

// getVideoEmbedURL - Obtiene la URL de embed según la fuente
func getVideoEmbedURL(video Video) string {
	if video.Source == "pornhub" {
		return "https://www.pornhub.com/embed/" + video.ID
	}
	return "https://www.eporner.com/embed/" + video.ID + "/"
}

// --- FETCHERS ---

func fetchEporner(query string, page int) []Video {
	if query == "" {
		query = "latest"
	}
	query = strings.ReplaceAll(query, " ", "+")
	url := fmt.Sprintf("https://www.eporner.com/api/v2/video/search/?query=%s&per_page=15&page=%d&thumbsize=medium", query, page)
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

func fetchPornhubRaw(query string, page int) []PHVideo {
	if query == "" {
		query = "recommended"
	}
	query = strings.ReplaceAll(query, " ", "+")
	url := fmt.Sprintf("https://www.pornhub.com/webmasters/search?search=%s&page=%d", query, page)
	resp, err := http.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var result PHResponse
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Videos
}

func normalizePH(raw []PHVideo) []Video {
	var normalized []Video
	for _, v := range raw {
		var tags []string
		for _, t := range v.Tags {
			tags = append(tags, t.TagName)
		}
		for _, p := range v.Pornstars {
			tags = append(tags, p.PornstarName)
		}

		normalized = append(normalized, Video{
			ID: v.VideoID, Title: v.Title, Views: v.Views,
			Rate: fmt.Sprintf("%.0f", v.Rating), LengthMin: v.Duration,
			Thumb: ThumbInfo{Src: v.DefaultThumb}, Keywords: strings.Join(tags, ", "),
			Source: "pornhub",
		})
	}
	return normalized
}

// --- MAIN ---

func main() {
	rand.Seed(time.Now().UnixNano())

	funcMap := template.FuncMap{
		"mod":           func(i, j int) bool { return i%j == 0 },
		"split":         strings.Split,
		"add":           add,
		"sub":           sub,
		"urlEncode":     urlEncode,
		"lastSeparator": lastSeparator,
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		page, _ := strconv.Atoi(r.URL.Query().Get("p"))
		if page < 1 {
			page = 1
		}

		var epVideos []Video
		var phRaw []PHVideo
		var wg sync.WaitGroup

		wg.Add(2)
		go func() { defer wg.Done(); epVideos = fetchEporner(query, page) }()
		go func() { defer wg.Done(); phRaw = fetchPornhubRaw(query, page) }()
		wg.Wait()

		phVideos := normalizePH(phRaw)
		allVideos := append(epVideos, phVideos...)
		rand.Shuffle(len(allVideos), func(i, j int) { allVideos[i], allVideos[j] = allVideos[j], allVideos[i] })

		// CATEGORÍAS DINÁMICAS BASADAS EN LOS VIDEOS ACTUALES
		dynamicCategories := extractDynamicMenu(phRaw)

		// Construir URL base del sitio
		baseURL := "https://mundoporn.com"

		tmpl := template.Must(template.New("layout.html").Funcs(funcMap).ParseFiles("templates/layout.html", "templates/index.html"))
		tmpl.ExecuteTemplate(w, "layout.html", IndexData{
			Videos:        allVideos,
			Categories:    dynamicCategories,
			CurrentPage:   page,
			NextPage:      page + 1,
			PrevPage:      page - 1,
			Query:         query,
			CurrentURL:    baseURL,
			CurrentTime:   time.Now().Format(time.RFC3339),
			IsVideo:       false,
			VideoEmbedURL: "",
		})
	})

	http.HandleFunc("/video/", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		title := r.URL.Query().Get("title")
		keywords := r.URL.Query().Get("keywords")
		rate := r.URL.Query().Get("rate")
		lengthMin := r.URL.Query().Get("len")
		viewsStr := r.URL.Query().Get("views")
		source := r.URL.Query().Get("source")
		thumbURL := r.URL.Query().Get("thumb")

		mainVideo := Video{
			ID:        id,
			Title:     title,
			Keywords:  keywords,
			Rate:      rate,
			LengthMin: lengthMin,
			Source:    source,
			Thumb:     ThumbInfo{Src: thumbURL},
		}
		mainVideo.Views, _ = strconv.Atoi(viewsStr)

		// Generar URL de embed
		videoEmbedURL := getVideoEmbedURL(mainVideo)

		searchQuery := "sexy"
		if mainVideo.Keywords != "" {
			searchQuery = strings.TrimSpace(strings.Split(mainVideo.Keywords, ",")[0])
		}

		// Para detalles también extraemos menú dinámico de los relacionados
		phRawRelated := fetchPornhubRaw(searchQuery, 1)
		related := append(fetchEporner(searchQuery, 1), normalizePH(phRawRelated)...)
		rand.Shuffle(len(related), func(i, j int) { related[i], related[j] = related[j], related[i] })

		// Construir URL base del sitio
		baseURL := "https://mundoporn.com/video/" + id

		tmpl := template.Must(template.New("layout.html").Funcs(funcMap).ParseFiles("templates/layout.html", "templates/details.html"))
		tmpl.ExecuteTemplate(w, "layout.html", DetailsData{
			MainVideo:     mainVideo,
			Related:       related,
			Categories:    extractDynamicMenu(phRawRelated),
			Query:         "",
			CurrentURL:    baseURL,
			CurrentTime:   time.Now().Format(time.RFC3339),
			IsVideo:       true,
			VideoEmbedURL: videoEmbedURL,
		})
	})

	// Esta línea hace que cuando alguien pida /sitemap.xml,
	// el servidor busque el archivo dentro de la carpeta public.
	http.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./public/sitemap.xml")
	})

	http.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "User-agent: *\nAllow: /\nSitemap: https://mundoporn.com/sitemap.xml")
	})

	// 1. Leer el puerto de la variable de entorno
	port := os.Getenv("PORT")

	// 2. Si está vacío (local), usar el 8080 o el que prefieras
	if port == "" {
		port = "8080"
	}

	fmt.Println("🚀 Servidor en http://localhost:8080")
	// Escuchar en "0.0.0.0:PORT" para que sea accesible externamente
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		panic(err)
	}
}
