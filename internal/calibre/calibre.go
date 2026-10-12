package calibre

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ErrContainerNotRunning is returned when the configured calibre
// container either doesn't exist or exists but isn't running.
var ErrContainerNotRunning = errors.New("calibre container is not running")

// AddError wraps a failed `calibredb add` invocation, carrying the
// captured stderr so the caller can surface it to the client/logs.
type AddError struct {
	ExitErr error
	Stderr  string
}

func (e *AddError) Error() string {
	return fmt.Sprintf("calibredb add failed: %v: %s", e.ExitErr, strings.TrimSpace(e.Stderr))
}

func (e *AddError) Unwrap() error { return e.ExitErr }

// Client execs into a named docker container to drive calibredb.
type Client struct {
	// ContainerName is the name of the running calibre container, e.g.
	// "calibre" (as shown by `docker ps --format '{{.Names}}'`).
	ContainerName string

	// Timeout bounds how long a single docker/calibredb invocation may
	// run before it's killed.
	Timeout time.Duration
}

// NewClient returns a Client for the given container name and
// per-call timeout.
func NewClient(containerName string, timeout time.Duration) *Client {
	return &Client{ContainerName: containerName, Timeout: timeout}
}

// EnsureRunning verifies that the configured container is currently
// running, via `docker inspect`. It returns ErrContainerNotRunning if
// the container doesn't exist or isn't running, distinct from
// unexpected docker/CLI failures, so callers can respond appropriately.
func (c *Client) EnsureRunning(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", c.ContainerName)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "No such") {
			return ErrContainerNotRunning
		}
		return fmt.Errorf("docker inspect %s: %w (stderr: %s)", c.ContainerName, err, strings.TrimSpace(stderr.String()))
	}

	if strings.TrimSpace(stdout.String()) != "true" {
		return ErrContainerNotRunning
	}
	return nil
}

// AddBook execs into the container and runs `calibredb add
// <containerPath>`, where containerPath is the location of the book as
// seen from *inside* the container (i.e. already mapped through the
// shared bind mount). It returns calibredb's stdout on success, or an
// *AddError on failure.
func (c *Client) AddBook(ctx context.Context, containerPath string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "exec", c.ContainerName, "calibredb", "add", containerPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", &AddError{ExitErr: err, Stderr: stderr.String()}
	}
	return stdout.String(), nil
}
