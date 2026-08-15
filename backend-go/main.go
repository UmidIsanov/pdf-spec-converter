package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// --- Загрузка .env ---
func loadDotEnv() {
	data, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func getAPIKey() string {
	return strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
}

// --- Хранилище фоновых задач ---
type Job struct {
	Status string      `json:"status"`           // pending | done
	Result *FileResult `json:"result,omitempty"`
}

var (
	jobs   = map[string]*Job{}
	jobsMu sync.Mutex
)

func newJobID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func setJob(id string, j *Job) {
	jobsMu.Lock()
	jobs[id] = j
	jobsMu.Unlock()
}

func takeJob(id string) (*Job, bool) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	j, ok := jobs[id]
	if ok && j.Status == "done" {
		delete(jobs, id) // отдаём результат и подчищаем
	}
	return j, ok
}

// --- Обработка одного файла в фоне ---
func processFile(id, filename string, raw []byte) {
	entry := &FileResult{Filename: filename, Items: []SpecItem{}}
	spec, err := parseSpecFromPDF(raw)
	if err != nil {
		msg := err.Error()
		entry.Error = &msg
		setJob(id, &Job{Status: "done", Result: entry})
		return
	}
	entry.DocNumber = spec.DocNumber
	entry.ObjectName = spec.ObjectName
	entry.SystemName = spec.SystemName
	if spec.Items != nil {
		entry.Items = spec.Items
	}
	setJob(id, &Job{Status: "done", Result: entry})
}

// --- HTTP-хендлеры ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "key_configured": getAPIKey() != ""})
}

func handleConvert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"detail": "только POST"})
		return
	}
	if getAPIKey() == "" {
		writeJSON(w, 500, map[string]string{"detail": "не настроен GEMINI_API_KEY"})
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeJSON(w, 400, map[string]string{"detail": "не удалось прочитать форму"})
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSON(w, 400, map[string]string{"detail": "нет файла"})
		return
	}
	fh := files[0] // фронтенд шлёт по одному файлу за запрос
	ff, err := fh.Open()
	if err != nil {
		writeJSON(w, 400, map[string]string{"detail": "не удалось открыть файл"})
		return
	}
	raw, err := io.ReadAll(ff)
	ff.Close()
	if err != nil {
		writeJSON(w, 400, map[string]string{"detail": "не удалось прочитать файл"})
		return
	}

	id := newJobID()
	setJob(id, &Job{Status: "pending"})
	go processFile(id, fh.Filename, raw)
	writeJSON(w, 200, map[string]string{"job_id": id})
}

func handleJob(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/job/")
	j, ok := takeJob(id)
	if !ok {
		writeJSON(w, 404, map[string]string{"detail": "Задача не найдена."})
		return
	}
	writeJSON(w, 200, j)
}

type exportRequest struct {
	Results []FileResult `json:"results"`
}

func handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"detail": "только POST"})
		return
	}
	var req exportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"detail": "неверный JSON"})
		return
	}
	xlsx, err := buildWorkbook(req.Results)
	if err != nil {
		writeJSON(w, 400, map[string]string{"detail": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="specification.xlsx"`)
	w.Write(xlsx)
}

// --- Раздача собранного фронтенда (SPA) ---
func staticHandler(dist string) http.HandlerFunc {
	fs := http.FileServer(http.Dir(dist))
	return func(w http.ResponseWriter, r *http.Request) {
		// если файла нет — отдаём index.html (SPA)
		p := filepath.Join(dist, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(p); err != nil || info.IsDir() {
			if r.URL.Path != "/" {
				if _, err := os.Stat(filepath.Join(dist, "index.html")); err == nil {
					http.ServeFile(w, r, filepath.Join(dist, "index.html"))
					return
				}
			}
		}
		fs.ServeHTTP(w, r)
	}
}

func main() {
	loadDotEnv()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8137"
	}
	dist := os.Getenv("FRONTEND_DIST")
	if dist == "" {
		dist = "frontend/dist"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/convert", handleConvert)
	mux.HandleFunc("/api/job/", handleJob)
	mux.HandleFunc("/api/export", handleExport)
	if _, err := os.Stat(dist); err == nil {
		mux.HandleFunc("/", staticHandler(dist))
	}

	addr := "127.0.0.1:" + port
	log.Printf("🚀 Go-бэкенд запущен на http://%s (фронт: %s)", addr, dist)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
