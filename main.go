package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"vk-post-parser/internal/config"
	"vk-post-parser/internal/downloader"
	"vk-post-parser/internal/models"
	"vk-post-parser/internal/parser"
	"vk-post-parser/internal/progress"
	"vk-post-parser/internal/vkapi"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", "error", err)
		fmt.Fprintf(os.Stderr, "\nUsage: vk-post-parser --community <id|domain> [--token <token>] [flags]\n")
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		fmt.Fprintf(os.Stderr, "  --community string     VK community ID or domain (required)\n")
		fmt.Fprintf(os.Stderr, "  --token string          VK API service token (or set VK_TOKEN env var)\n")
		fmt.Fprintf(os.Stderr, "  --output string         Output JSON file path (default \"posts.json\")\n")
		fmt.Fprintf(os.Stderr, "  --images-dir string     Directory for downloaded images (default \"images\")\n")
		fmt.Fprintf(os.Stderr, "  --no-download           Disable image downloading\n")
		fmt.Fprintf(os.Stderr, "  --workers int           Concurrent image download workers (default 5)\n")
		fmt.Fprintf(os.Stderr, "  --batch-size int        Execute batch size, max 25 (default 25)\n")
		fmt.Fprintf(os.Stderr, "  --progress-file string  Progress file path (default \".vk-parser-progress.json\")\n")
		os.Exit(1)
	}

	// Graceful shutdown context
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Resolve community: numeric ID or domain
	ownerID, domain := resolveCommunity(cfg.Community)

	client := vkapi.NewClient(cfg.Token, cfg.APIVersion, logger)

	// Load progress for resume
	state, err := progress.Load(cfg.ProgressFile)
	if err != nil {
		logger.Error("failed to load progress", "error", err)
		os.Exit(1)
	}

	// Check if a different community is being resumed
	if state.FetchedCount > 0 && state.Community != cfg.Community {
		logger.Warn("progress file is for a different community, starting fresh",
			"progress_community", state.Community,
			"requested_community", cfg.Community)
		state = &progress.State{}
	}

	if state.Completed {
		logger.Info("previous run completed, starting fresh")
		state = &progress.State{}
	}

	state.Community = cfg.Community

	// Load existing posts if resuming
	var allPosts []models.Post
	if state.FetchedCount > 0 {
		allPosts, err = loadExistingPosts(cfg.OutputFile)
		if err != nil {
			logger.Warn("failed to load existing posts, starting fresh", "error", err)
			state = &progress.State{Community: cfg.Community}
			allPosts = nil
		} else {
			state.FetchedCount = len(allPosts)
			logger.Info("resuming previous run",
				"existing_posts", len(allPosts),
				"last_offset", state.LastOffset,
				"total", state.TotalPosts,
				"percent", formatPercent(len(allPosts), state.TotalPosts))
		}
	}

	// Set up image downloader
	dl := downloader.New(cfg.ImagesDir, cfg.DownloadWorkers, logger)
	if cfg.DownloadImages {
		if err := dl.EnsureDir(); err != nil {
			logger.Error("failed to create images directory", "error", err)
			os.Exit(1)
		}
	}

	statsMu := sync.Mutex{}
	stats := progressStats{
		postsFetched: len(allPosts),
		postsTotal:   state.TotalPosts,
		lastOffset:   state.LastOffset,
	}

	progressStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-progressStop:
				return
			case <-ticker.C:
				s := snapshotProgress(&statsMu, &stats)
				logger.Info("progress_tick",
					"fetched", s.postsFetched,
					"total", s.postsTotal,
					"percent", formatPercent(s.postsFetched, s.postsTotal),
					"last_offset", s.lastOffset,
					"images_done", s.imagesDone,
					"images_total", s.imagesTotal)
			}
		}
	}()
	defer close(progressStop)

	// Fetch all posts
	err = client.FetchAllPosts(ctx, ownerID, domain, state.LastOffset, cfg.ExecuteBatchSize,
		func(posts []vkapi.WallPost, total int, offset int, nextOffset int) error {
			parsed := parser.ParsePosts(posts)

			batchImagesTotal := 0
			batchImagesDone := 0
			// Download images
			if cfg.DownloadImages {
				tasks := buildDownloadTasks(parsed)
				batchImagesTotal = len(tasks)
				results := dl.Download(ctx, tasks)
				batchImagesDone = countSuccessfulDownloads(results)
				applyDownloadResults(parsed, results)
			}

			allPosts = append(allPosts, parsed...)

			// Update and save progress
			state.FetchedCount = len(allPosts)
			state.LastOffset = nextOffset
			state.TotalPosts = total
			if err := progress.Save(cfg.ProgressFile, state); err != nil {
				logger.Error("failed to save progress", "error", err)
			}

			// Save posts to JSON
			if err := savePostsJSON(cfg.OutputFile, cfg.Community, allPosts); err != nil {
				logger.Error("failed to save posts JSON", "error", err)
				return err
			}

			logger.Info("progress",
				"fetched", len(allPosts),
				"total", total,
				"percent", formatPercent(len(allPosts), total),
				"offset", offset,
				"batch_posts", len(parsed),
				"images_done_batch", batchImagesDone,
				"images_total_batch", batchImagesTotal)

			updateProgressStats(&statsMu, &stats, len(allPosts), total, nextOffset, batchImagesDone, batchImagesTotal)

			return nil
		},
	)

	if err != nil {
		if ctx.Err() != nil {
			logger.Info("interrupted, progress saved",
				"fetched", len(allPosts),
				"last_offset", state.LastOffset)
		} else {
			logger.Error("fetch failed", "error", err)
		}
		os.Exit(1)
	}

	// Mark as completed
	state.Completed = true
	if err := progress.Save(cfg.ProgressFile, state); err != nil {
		logger.Error("failed to save final progress", "error", err)
	}

	// Deduplicate by post ID (in case of resume overlap)
	allPosts = deduplicatePosts(allPosts)

	// Final save
	if err := savePostsJSON(cfg.OutputFile, cfg.Community, allPosts); err != nil {
		logger.Error("failed to save final JSON", "error", err)
		os.Exit(1)
	}

	logger.Info("done",
		"total_posts", len(allPosts),
		"output", cfg.OutputFile)
}

func resolveCommunity(community string) (ownerID int, domain string) {
	// Try to parse as numeric ID
	id, err := strconv.Atoi(community)
	if err == nil {
		// Ensure negative for community
		if id > 0 {
			id = -id
		}
		return id, ""
	}
	return 0, community
}

func loadExistingPosts(path string) ([]models.Post, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var output models.Output
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, err
	}

	return output.Posts, nil
}

func savePostsJSON(path, community string, posts []models.Post) error {
	output := models.Output{
		Community:  community,
		TotalPosts: len(posts),
		FetchedAt:  time.Now().UTC().Format(time.RFC3339),
		Posts:      posts,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename JSON: %w", err)
	}

	return nil
}

func buildDownloadTasks(posts []models.Post) []downloader.Task {
	var tasks []downloader.Task
	for i := range posts {
		for j, photo := range posts[i].Photos {
			ext := extractExt(photo.URL)
			filename := fmt.Sprintf("wall%d_%d_%d%s", posts[i].OwnerID, posts[i].ID, j, ext)
			tasks = append(tasks, downloader.Task{
				URL:      photo.URL,
				Filename: filename,
			})
		}
		// Also handle repost photos
		if posts[i].RepostOf != nil {
			for j, photo := range posts[i].RepostOf.Photos {
				ext := extractExt(photo.URL)
				filename := fmt.Sprintf("wall%d_%d_repost_%d%s", posts[i].OwnerID, posts[i].ID, j, ext)
				tasks = append(tasks, downloader.Task{
					URL:      photo.URL,
					Filename: filename,
				})
			}
		}
	}
	return tasks
}

func applyDownloadResults(posts []models.Post, results []downloader.Result) {
	resultMap := make(map[string]string, len(results))
	for _, r := range results {
		if r.Err == nil && r.LocalPath != "" {
			resultMap[r.Task.URL] = r.LocalPath
		}
	}

	for i := range posts {
		for j := range posts[i].Photos {
			if localPath, ok := resultMap[posts[i].Photos[j].URL]; ok {
				posts[i].Photos[j].LocalPath = localPath
			}
		}
		if posts[i].RepostOf != nil {
			for j := range posts[i].RepostOf.Photos {
				if localPath, ok := resultMap[posts[i].RepostOf.Photos[j].URL]; ok {
					posts[i].RepostOf.Photos[j].LocalPath = localPath
				}
			}
		}
	}
}

func extractExt(url string) string {
	// Extract file extension from URL, default to .jpg
	parts := strings.Split(url, "?")
	path := parts[0]
	ext := filepath.Ext(path)
	if ext == "" || len(ext) > 5 {
		return ".jpg"
	}
	return ext
}

func deduplicatePosts(posts []models.Post) []models.Post {
	seen := make(map[string]struct{}, len(posts))
	result := make([]models.Post, 0, len(posts))
	for _, p := range posts {
		key := fmt.Sprintf("%d_%d", p.OwnerID, p.ID)
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			result = append(result, p)
		}
	}
	return result
}

type progressStats struct {
	postsFetched int
	postsTotal   int
	lastOffset   int
	imagesDone   int
	imagesTotal  int
}

func updateProgressStats(mu *sync.Mutex, stats *progressStats, postsFetched, postsTotal, lastOffset, imagesDone, imagesTotal int) {
	mu.Lock()
	defer mu.Unlock()
	stats.postsFetched = postsFetched
	stats.postsTotal = postsTotal
	stats.lastOffset = lastOffset
	stats.imagesDone += imagesDone
	stats.imagesTotal += imagesTotal
}

func snapshotProgress(mu *sync.Mutex, stats *progressStats) progressStats {
	mu.Lock()
	defer mu.Unlock()
	return *stats
}

func formatPercent(fetched, total int) string {
	if total <= 0 {
		return "0.0%"
	}
	pct := float64(fetched) / float64(total) * 100
	return fmt.Sprintf("%.1f%%", pct)
}

func countSuccessfulDownloads(results []downloader.Result) int {
	count := 0
	for _, r := range results {
		if r.Err == nil && r.LocalPath != "" {
			count++
		}
	}
	return count
}
