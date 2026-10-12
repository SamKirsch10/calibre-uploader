package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	// ListenAddr is the address the HTTP server binds to.
	ListenAddr string

	// HostUploadDir is where this process writes uploaded files, as seen
	// on its own filesystem.
	HostUploadDir string

	// ContainerUploadDir is the path at which the SAME directory
	// (HostUploadDir) is bind-mounted inside the calibre container. The
	// two must point at the same bind mount or calibredb add will be
	// told to add a file that doesn't exist from its point of view.
	ContainerUploadDir string

	// CalibreContainerName is the name of the running calibre container,
	// e.g. "calibre" (as shown by `docker ps --format '{{.Names}}'`).
	CalibreContainerName string

	// MaxUploadBytes caps the size of an accepted upload.
	MaxUploadBytes int64

	// CalibredbTimeout bounds how long a single `docker exec ... calibredb
	// add` invocation may run before it's killed.
	CalibredbTimeout time.Duration

	// DeleteAfterAdd removes the uploaded file from HostUploadDir once
	// calibredb add has successfully imported it, since calibre keeps its
	// own copy in the library and there's no reason to keep two.
	DeleteAfterAdd bool
}

// Load builds a Config from environment variables, applying defaults
// for anything unset.
func Load() (Config, error) {
	cfg := Config{
		ListenAddr:           getEnv("LISTEN_ADDR", ":8080"),
		HostUploadDir:        getEnv("HOST_UPLOAD_DIR", "./uploads"),
		ContainerUploadDir:   getEnv("CONTAINER_UPLOAD_DIR", "/uploads"),
		CalibreContainerName: getEnv("CALIBRE_CONTAINER_NAME", "calibre"),
		DeleteAfterAdd:       getEnvBool("DELETE_AFTER_ADD", false),
	}

	maxMB, err := getEnvInt("MAX_UPLOAD_MB", 200)
	if err != nil {
		return Config{}, err
	}
	cfg.MaxUploadBytes = int64(maxMB) * 1024 * 1024

	timeoutSec, err := getEnvInt("CALIBREDB_TIMEOUT_SECONDS", 120)
	if err != nil {
		return Config{}, err
	}
	cfg.CalibredbTimeout = time.Duration(timeoutSec) * time.Second

	if cfg.CalibreContainerName == "" {
		return Config{}, fmt.Errorf("CALIBRE_CONTAINER_NAME must not be empty")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvInt(key string, fallback int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %w", key, err)
	}
	return n, nil
}
