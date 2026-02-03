# VK Post Parser

CLI tool to download posts from a VK community into a JSON file, with optional image downloading and resume support.

## Features
- Fetch all wall posts for a community (ID or domain)
- Save results to `posts.json`
- Optional image downloads with concurrency control
- Resume support via `.vk-parser-progress.json`

## Requirements
- Go 1.25+ (for building)
- VK API token (service token recommended)

Token docs: [VK access token getting started](https://dev.vk.com/ru/api/access-token/getting-started)

## Build
```
go build -o vk-post-parser .
```

## Usage
Basic run:
```
./vk-post-parser --community <id|domain> --token <TOKEN>
```

Example (no image downloads):
```
./vk-post-parser --community sample_community --token TOKEN --no-download
```

Using env var for token:
```
VK_TOKEN=TOKEN ./vk-post-parser --community <id|domain>
```

## Flags
- `--community` (required) VK community ID or domain
- `--token` VK API service token (or set `VK_TOKEN`)
- `--output` Output JSON file path (default `posts.json`)
- `--images-dir` Directory for downloaded images (default `images`)
- `--no-download` Disable image downloading
- `--workers` Concurrent image download workers (default 5)
- `--batch-size` Execute batch size (max 25, default 25)
- `--progress-file` Progress file path (default `.vk-parser-progress.json`)

## Output
`posts.json` contains the parsed posts with metadata, attachments, and optional local image paths if downloads are enabled.

## Example output (sanitized)
The snippet below shows lines of `posts.json` with anonymized data.
```json
{
  "community": "sample_community",
  "total_posts": 1,
  "fetched_at": "2020-01-01T00:00:00Z",
  "posts": [
    {
      "id": 0,
      "owner_id": 0,
      "url": "https://example.com/post/0",
      "text": "Sample text",
      "date_unix": 0,
      "date_human": "2020-01-01T00:00:00Z",
      "post_type": "post",
      "is_pinned": true,
      "photos": [
        {
          "url": "https://example.com/post/0",
          "width": 0,
          "height": 0
        }
      ],
      "likes": 0,
      "reposts": 0,
      "comments": 0,
      "views": 0,
      "is_repost": false
    }
  ]
}
```

## Resume
The parser writes `.vk-parser-progress.json` after each batch. If interrupted, re-run the command with the same community to resume.
