package config

import (
	"flag"
	"fmt"
	"os"
)

type Config struct {
	Token            string
	Community        string
	APIVersion       string
	OutputFile       string
	ImagesDir        string
	DownloadWorkers  int
	DownloadImages   bool
	ExecuteBatchSize int
	ProgressFile     string
}

func Load() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.Token, "token", "", "VK API service token (or set VK_TOKEN env var)")
	flag.StringVar(&cfg.Community, "community", "", "VK community ID or domain (required)")
	flag.StringVar(&cfg.APIVersion, "api-version", "5.199", "VK API version")
	flag.StringVar(&cfg.OutputFile, "output", "posts.json", "Output JSON file path")
	flag.StringVar(&cfg.ImagesDir, "images-dir", "images", "Directory for downloaded images")
	flag.IntVar(&cfg.DownloadWorkers, "workers", 5, "Concurrent image download workers")
	noDownload := flag.Bool("no-download", false, "Disable image downloading")
	flag.IntVar(&cfg.ExecuteBatchSize, "batch-size", 25, "Number of wall.get calls per execute batch (max 25)")
	flag.StringVar(&cfg.ProgressFile, "progress-file", ".vk-parser-progress.json", "Progress file for resume")

	flag.Parse()

	cfg.DownloadImages = !*noDownload

	if cfg.Token == "" {
		cfg.Token = os.Getenv("VK_TOKEN")
	}

	if cfg.ExecuteBatchSize < 1 || cfg.ExecuteBatchSize > 25 {
		cfg.ExecuteBatchSize = 25
	}

	if cfg.Token == "" {
		return nil, fmt.Errorf("VK API token is required: use --token flag or VK_TOKEN env var")
	}
	if cfg.Community == "" {
		return nil, fmt.Errorf("community is required: use --community flag")
	}

	return cfg, nil
}
