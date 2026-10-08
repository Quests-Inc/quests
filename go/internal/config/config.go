package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Root    string
	DataDir string
	DBPath  string
	// DatabaseURL (QUESTS_DATABASE_URL, postgres://…) switches from the SQLite
	// file at DBPath to PostgreSQL.
	DatabaseURL string
	Host        string
	Port        int
	CORS        []string
	// AuthMode is "auto" (enforce as soon as an account exists), "on" or "off".
	AuthMode string
	// SecureCookies marks the session cookie Secure. Enable behind HTTPS.
	SecureCookies bool

	// WebDAV backs attachments. Quests stores only metadata; the bytes live
	// here. Empty WebDAVURL disables attachment uploads entirely.
	WebDAVURL  string
	WebDAVUser string
	WebDAVPass string
	// ClamAVAddr is host:port of clamd. Empty means no scanning is available,
	// which refuses uploads rather than accepting unscanned files.
	ClamAVAddr string
	// MaxUploadBytes caps a single attachment.
	MaxUploadBytes int64

	// BackupInterval is how often a DB snapshot is taken. <=0 disables it.
	BackupInterval time.Duration
	// BackupKeep is how many snapshots to retain (oldest pruned first),
	// both on disk and on WebDAV / remote PC (when configured).
	BackupKeep int

	// BackupRemote* push DB + attachments archive to a second host (PC) via SFTP.
	// Empty BackupRemoteHost disables the remote step.
	BackupRemoteHost string
	BackupRemoteDir  string
	BackupRemoteKey  string
	BackupRemotePort int
}

func Load() Config {
	root := strings.TrimSpace(os.Getenv("QUESTS_ROOT"))
	if root == "" {
		root = findRepoRoot()
	}
	data := strings.TrimSpace(os.Getenv("QUESTS_DATA_DIR"))
	if data == "" {
		data = filepath.Join(root, "data")
	}
	host := strings.TrimSpace(os.Getenv("QUESTS_HOST"))
	if host == "" {
		host = "127.0.0.1"
	}
	port := 8765
	if raw := strings.TrimSpace(os.Getenv("QUESTS_PORT")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			port = n
		}
	}
	cors := []string{"http://127.0.0.1:5173", "http://localhost:5173"}
	if raw := strings.TrimSpace(os.Getenv("QUESTS_CORS_ORIGINS")); raw != "" {
		cors = nil
		for _, o := range strings.Split(raw, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				cors = append(cors, o)
			}
		}
	}
	authMode := strings.ToLower(strings.TrimSpace(os.Getenv("QUESTS_AUTH")))
	switch authMode {
	case "on", "off", "auto":
	case "1", "true", "yes":
		authMode = "on"
	case "0", "false", "no":
		authMode = "off"
	default:
		authMode = "auto"
	}
	secure := false
	switch strings.ToLower(strings.TrimSpace(os.Getenv("QUESTS_SECURE_COOKIES"))) {
	case "1", "true", "yes", "on":
		secure = true
	}
	maxUpload := int64(25 << 20)
	if raw := strings.TrimSpace(os.Getenv("QUESTS_MAX_UPLOAD_MB")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			maxUpload = int64(n) << 20
		}
	}
	backupInterval := 24 * time.Hour
	if raw := strings.TrimSpace(os.Getenv("QUESTS_BACKUP_INTERVAL_HOURS")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			backupInterval = time.Duration(n) * time.Hour
		}
	}
	backupKeep := 14
	if raw := strings.TrimSpace(os.Getenv("QUESTS_BACKUP_KEEP")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			backupKeep = n
		}
	}
	remotePort := 22
	if raw := strings.TrimSpace(os.Getenv("QUESTS_BACKUP_REMOTE_PORT")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			remotePort = n
		}
	}
	remoteKey := strings.TrimSpace(os.Getenv("QUESTS_BACKUP_REMOTE_KEY"))
	if remoteKey == "" {
		remoteKey = filepath.Join(data, "backup_ssh_key")
	}
	return Config{
		AuthMode:         authMode,
		SecureCookies:    secure,
		Root:             root,
		DataDir:          data,
		DBPath:           filepath.Join(data, "quests.db"),
		DatabaseURL:      strings.TrimSpace(os.Getenv("QUESTS_DATABASE_URL")),
		Host:             host,
		Port:             port,
		CORS:             cors,
		WebDAVURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("QUESTS_WEBDAV_URL")), "/"),
		WebDAVUser:       strings.TrimSpace(os.Getenv("QUESTS_WEBDAV_USER")),
		WebDAVPass:       os.Getenv("QUESTS_WEBDAV_PASS"),
		ClamAVAddr:       strings.TrimSpace(os.Getenv("QUESTS_CLAMAV_ADDR")),
		MaxUploadBytes:   maxUpload,
		BackupInterval:   backupInterval,
		BackupKeep:       backupKeep,
		BackupRemoteHost: strings.TrimSpace(os.Getenv("QUESTS_BACKUP_REMOTE_HOST")),
		BackupRemoteDir:  strings.TrimSpace(os.Getenv("QUESTS_BACKUP_REMOTE_DIR")),
		BackupRemoteKey:  remoteKey,
		BackupRemotePort: remotePort,
	}
}

func findRepoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "go", "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return wd
}

func (c Config) Addr() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

// IsLoopbackBind reports whether the server only accepts local connections.
// A non-loopback bind with no accounts is refused at startup.
func (c Config) IsLoopbackBind() bool {
	switch c.Host {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	return false
}
