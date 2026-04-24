package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
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

type IndexData struct {
	Videos      []Video
	Categories  []string
	CurrentPage int
	NextPage    int
	PrevPage    int
	Query       string
}

type DetailsData struct {
	MainVideo  Video
	Related    []Video
	Categories []string
	Query      string
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
		"mod":   func(i, j int) bool { return i%j == 0 },
		"split": strings.Split,
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

		tmpl := template.Must(template.New("layout.html").Funcs(funcMap).ParseFiles("templates/layout.html", "templates/index.html"))
		tmpl.ExecuteTemplate(w, "layout.html", IndexData{
			Videos: allVideos, Categories: dynamicCategories,
			CurrentPage: page, NextPage: page + 1, PrevPage: page - 1, Query: query,
		})
	})

	http.HandleFunc("/video/", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		mainVideo := Video{
			ID: id, Title: r.URL.Query().Get("title"),
			Keywords: r.URL.Query().Get("keywords"), Rate: r.URL.Query().Get("rate"),
			LengthMin: r.URL.Query().Get("len"), Source: r.URL.Query().Get("source"),
			Thumb: ThumbInfo{Src: r.URL.Query().Get("thumb")},
		}
		mainVideo.Views, _ = strconv.Atoi(r.URL.Query().Get("views"))

		searchQuery := "sexy"
		if mainVideo.Keywords != "" {
			searchQuery = strings.TrimSpace(strings.Split(mainVideo.Keywords, ",")[0])
		}

		// Para detalles también extraemos menú dinámico de los relacionados
		phRawRelated := fetchPornhubRaw(searchQuery, 1)
		related := append(fetchEporner(searchQuery, 1), normalizePH(phRawRelated)...)
		rand.Shuffle(len(related), func(i, j int) { related[i], related[j] = related[j], related[i] })

		tmpl := template.Must(template.New("layout.html").Funcs(funcMap).ParseFiles("templates/layout.html", "templates/details.html"))
		tmpl.ExecuteTemplate(w, "layout.html", DetailsData{
			MainVideo: mainVideo, Related: related,
			Categories: extractDynamicMenu(phRawRelated), Query: "",
		})
	})

	// Esta línea hace que cuando alguien pida /sitemap.xml,
	// el servidor busque el archivo dentro de la carpeta public.
	http.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./public/sitemap.xml")
	})

	http.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "User-agent: *\nAllow: /\nSitemap: http://localhost:8080/sitemap.xml")
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
