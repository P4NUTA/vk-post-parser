package downloader

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Task represents a single image download task.
type Task struct {
	URL      string
	Filename string
}

// Result is the outcome of a download task.
type Result struct {
	Task      Task
	LocalPath string
	Err       error
}

// Downloader handles concurrent image downloading.
type Downloader struct {
	dir        string
	workers    int
	httpClient *http.Client
	logger     *slog.Logger
}

// New creates a new Downloader.
func New(dir string, workers int, logger *slog.Logger) *Downloader {
	return &Downloader{
		dir:     dir,
		workers: workers,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		logger: logger,
	}
}

// EnsureDir creates the images directory if it doesn't exist.
func (d *Downloader) EnsureDir() error {
	return os.MkdirAll(d.dir, 0o755)
}

// Download downloads all tasks concurrently with a worker limit.
func (d *Downloader) Download(ctx context.Context, tasks []Task) []Result {
	if len(tasks) == 0 {
		return nil
	}

	results := make([]Result, len(tasks))
	sem := make(chan struct{}, d.workers)
	var wg sync.WaitGroup

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t Task) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				results[idx] = Result{Task: t, Err: ctx.Err()}
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			localPath := filepath.Join(d.dir, t.Filename)

			// Skip if already downloaded
			if _, err := os.Stat(localPath); err == nil {
				results[idx] = Result{Task: t, LocalPath: localPath}
				return
			}

			err := d.downloadFile(ctx, t.URL, localPath)
			if err != nil {
				d.logger.Warn("image download failed", "url", t.URL, "error", err)
				results[idx] = Result{Task: t, Err: err}
				return
			}

			results[idx] = Result{Task: t, LocalPath: localPath}
		}(i, task)
	}

	wg.Wait()
	return results
}

func (d *Downloader) downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Write to temp file, then rename for atomicity
	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	_, err = io.Copy(f, resp.Body)
	closeErr := f.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write file: %w", err)
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close file: %w", closeErr)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename file: %w", err)
	}

	return nil
}
