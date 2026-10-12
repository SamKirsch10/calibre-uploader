package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"calibre-uploader/internal/calibre"
	"calibre-uploader/internal/ebook"
)

// uploadResponse is the JSON body returned for both success and
// failure, so clients always get the same shape back.
type uploadResponse struct {
	OK              bool   `json:"ok"`
	Message         string `json:"message"`
	SavedFilename   string `json:"saved_filename,omitempty"`
	CalibredbOutput string `json:"calibredb_output,omitempty"`
}

var unsafeFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// sanitizeFilename strips any directory components and replaces
// anything that isn't alphanumeric/dot/dash/underscore, so the result
// is safe to use as a bare filename on disk (no path traversal, no
// surprises from spaces/unicode/control characters).
func sanitizeFilename(name string) string {
	base := filepath.Base(name)
	base = unsafeFilenameChars.ReplaceAllString(base, "_")
	base = strings.TrimLeft(base, ".") // avoid resurrecting "." / ".." or dotfiles
	if base == "" {
		base = "upload"
	}
	return base
}

// uniqueSuffix returns a short random hex string used to avoid
// filename collisions between concurrent uploads of same-named files.
func uniqueSuffix() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, uploadResponse{OK: false, Message: fmt.Sprintf("failed to parse upload (too large or malformed): %v", err)})
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		fileFields := make([]string, 0, len(r.MultipartForm.File))
		for name, headers := range r.MultipartForm.File {
			for _, h := range headers {
				fileFields = append(fileFields, fmt.Sprintf("%s=%q(%s)", name, h.Filename, h.Header.Get("Content-Type")))
			}
		}
		valueFields := make([]string, 0, len(r.MultipartForm.Value))
		for name, values := range r.MultipartForm.Value {
			valueFields = append(valueFields, fmt.Sprintf("%s=%q", name, values))
		}
		log.Printf("upload missing \"file\" field: err=%v content-type=%q file-fields=%v value-fields=%v",
			err, r.Header.Get("Content-Type"), fileFields, valueFields)
		writeJSON(w, http.StatusBadRequest, uploadResponse{OK: false, Message: "missing \"file\" form field"})
		return
	}
	defer file.Close()

	// Peek the header for magic-byte validation without losing those
	// bytes for the subsequent write to disk.
	sniffBuf := make([]byte, ebook.SniffLen)
	n, err := io.ReadFull(file, sniffBuf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		writeJSON(w, http.StatusBadRequest, uploadResponse{OK: false, Message: fmt.Sprintf("failed to read upload: %v", err)})
		return
	}
	sniffBuf = sniffBuf[:n]

	if err := ebook.Validate(fileHeader.Filename, sniffBuf); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, uploadResponse{OK: false, Message: err.Error()})
		return
	}

	safeName := sanitizeFilename(fileHeader.Filename)
	suffix, err := uniqueSuffix()
	if err != nil {
		log.Printf("failed to generate unique suffix: %v", err)
		writeJSON(w, http.StatusInternalServerError, uploadResponse{OK: false, Message: "internal error"})
		return
	}
	ext := filepath.Ext(safeName)
	base := strings.TrimSuffix(safeName, ext)
	savedName := fmt.Sprintf("%s-%d-%s%s", base, time.Now().Unix(), suffix, ext)

	hostPath := filepath.Join(s.cfg.HostUploadDir, savedName)
	out, err := os.OpenFile(hostPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("failed to create %s: %v", hostPath, err)
		writeJSON(w, http.StatusInternalServerError, uploadResponse{OK: false, Message: "failed to store upload"})
		return
	}

	writeErr := func() error {
		defer out.Close()
		if _, err := out.Write(sniffBuf); err != nil {
			return err
		}
		_, err := io.Copy(out, file)
		return err
	}()
	if writeErr != nil {
		log.Printf("failed to write %s: %v", hostPath, writeErr)
		os.Remove(hostPath)
		writeJSON(w, http.StatusInternalServerError, uploadResponse{OK: false, Message: "failed to store upload"})
		return
	}

	log.Printf("stored upload %q (%d bytes) -> %s", fileHeader.Filename, fileHeader.Size, hostPath)

	containerPath := path.Join(s.cfg.ContainerUploadDir, savedName)

	if err := s.calibre.EnsureRunning(r.Context()); err != nil {
		if errors.Is(err, calibre.ErrContainerNotRunning) {
			log.Printf("calibre container %q not running; leaving file at %s for retry", s.cfg.CalibreContainerName, hostPath)
			writeJSON(w, http.StatusServiceUnavailable, uploadResponse{
				OK:            false,
				Message:       fmt.Sprintf("calibre container %q is not running; file was saved to %s and can be retried", s.cfg.CalibreContainerName, hostPath),
				SavedFilename: savedName,
			})
			return
		}
		log.Printf("error checking calibre container: %v", err)
		writeJSON(w, http.StatusInternalServerError, uploadResponse{OK: false, Message: "failed to reach docker"})
		return
	}

	output, err := s.calibre.AddBook(r.Context(), containerPath)
	if err != nil {
		log.Printf("calibredb add failed for %s: %v", containerPath, err)
		writeJSON(w, http.StatusBadGateway, uploadResponse{
			OK:            false,
			Message:       fmt.Sprintf("calibredb add failed: %v", err),
			SavedFilename: savedName,
		})
		return
	}

	log.Printf("calibredb add succeeded for %s", containerPath)

	if s.cfg.DeleteAfterAdd {
		if err := os.Remove(hostPath); err != nil {
			log.Printf("warning: failed to remove %s after successful import: %v", hostPath, err)
		}
	}

	writeJSON(w, http.StatusOK, uploadResponse{
		OK:              true,
		Message:         "book imported successfully",
		SavedFilename:   savedName,
		CalibredbOutput: strings.TrimSpace(output),
	})
}

func writeJSON(w http.ResponseWriter, status int, body uploadResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}
