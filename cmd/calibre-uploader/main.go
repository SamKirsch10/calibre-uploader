package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"calibre-uploader/internal/api"
	"calibre-uploader/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	if err := os.MkdirAll(cfg.HostUploadDir, 0o755); err != nil {
		log.Fatalf("failed to create upload dir %s: %v", cfg.HostUploadDir, err)
	}

	srv := api.NewServer(cfg)

	httpServer := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      srv.Routes(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: cfg.CalibredbTimeout + 30*time.Second,
	}

	log.Printf("calibre-uploader listening on %s", cfg.ListenAddr)
	log.Printf("config: host_upload_dir=%s container_upload_dir=%s calibre_container=%s delete_after_add=%v",
		cfg.HostUploadDir, cfg.ContainerUploadDir, cfg.CalibreContainerName, cfg.DeleteAfterAdd)

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
