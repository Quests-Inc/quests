package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Speech recognition is configured once on this server and called from here.
// The browser never talks to the recognizer: a page on another origin cannot
// reach a LAN address, and the key stays on the host.

type sttFile struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
}

func (s *Server) sttPath() string {
	return filepath.Join(s.DataDir, "stt.json")
}

func (s *Server) readSTT() sttFile {
	cfg := sttFile{}
	if raw, err := os.ReadFile(s.sttPath()); err == nil {
		_ = json.Unmarshal(raw, &cfg)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = strings.TrimSpace(os.Getenv("QUESTS_STT_BASE"))
	}
	if cfg.Model == "" {
		if m := strings.TrimSpace(os.Getenv("QUESTS_STT_MODEL")); m != "" {
			cfg.Model = m
		} else {
			cfg.Model = "whisper-1"
		}
	}
	if cfg.APIKey == "" {
		cfg.APIKey = strings.TrimSpace(os.Getenv("QUESTS_STT_KEY"))
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	return cfg
}

func (s *Server) writeSTT(cfg sttFile) error {
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.DataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.sttPath(), raw, 0o600)
}

// checkSTTURL accepts http and https, including a recognizer on the LAN.
// The address is stored by the person who runs this server, then called only by it.
func checkSTTURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", errSTTURL
	}
	return raw, nil
}

var errSTTURL = errString("адрес распознавания: нужен http(s) без логина в ссылке")

type errString string

func (e errString) Error() string { return string(e) }

func (s *Server) getSTT(w http.ResponseWriter, r *http.Request) {
	cfg := s.readSTT()
	writeJSON(w, http.StatusOK, map[string]any{
		"base_url": cfg.BaseURL,
		"model":    cfg.Model,
		"has_key":  cfg.APIKey != "",
		"enabled":  cfg.BaseURL != "",
	})
}

func (s *Server) putSTT(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BaseURL string  `json:"base_url"`
		Model   string  `json:"model"`
		APIKey  *string `json:"api_key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	base, err := checkSTTURL(in.BaseURL)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	model := strings.TrimSpace(in.Model)
	if model == "" {
		model = "whisper-1"
	}
	if len(model) > 100 {
		writeErr(w, http.StatusUnprocessableEntity, "модель: не длиннее 100 символов")
		return
	}
	cfg := s.readSTT()
	cfg.BaseURL = base
	cfg.Model = model
	if in.APIKey != nil {
		cfg.APIKey = strings.TrimSpace(*in.APIKey)
	}
	if err := s.writeSTT(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.getSTT(w, r)
}

func (s *Server) postSTT(w http.ResponseWriter, r *http.Request) {
	cfg := s.readSTT()
	if cfg.BaseURL == "" {
		writeErr(w, http.StatusConflict, "распознавание не настроено")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "нужен файл записи")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "нужен файл записи")
		return
	}
	defer file.Close()
	audio, err := io.ReadAll(io.LimitReader(file, 32<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", hdr.Filename)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := part.Write(audio); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = mw.WriteField("model", cfg.Model)
	lang := strings.TrimSpace(r.FormValue("language"))
	if lang == "" {
		lang = "ru"
	}
	_ = mw.WriteField("language", lang)
	_ = mw.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL+"/v1/audio/transcriptions", &body)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "распознавание недоступно: "+err.Error())
		return
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		writeErr(w, http.StatusBadGateway, strings.TrimSpace(string(out)))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}
