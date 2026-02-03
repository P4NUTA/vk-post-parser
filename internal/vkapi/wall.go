package vkapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// FetchWallCount fetches the total number of posts on the wall.
func (c *Client) FetchWallCount(ctx context.Context, ownerID int, domain string) (int, error) {
	params := url.Values{
		"count": {"1"},
	}
	setOwnerParam(params, ownerID, domain)

	raw, err := c.RequestWithRetry(ctx, "wall.get", params)
	if err != nil {
		return 0, fmt.Errorf("fetch wall count: %w", err)
	}

	var resp WallGetResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return 0, fmt.Errorf("unmarshal wall count: %w", err)
	}

	return resp.Count, nil
}

// FetchPostsBatch fetches a batch of posts using a single wall.get call.
func (c *Client) FetchPostsBatch(ctx context.Context, ownerID int, domain string, offset, count int) (*WallGetResponse, error) {
	params := url.Values{
		"count":  {strconv.Itoa(count)},
		"offset": {strconv.Itoa(offset)},
	}
	setOwnerParam(params, ownerID, domain)

	raw, err := c.RequestWithRetry(ctx, "wall.get", params)
	if err != nil {
		return nil, fmt.Errorf("fetch posts batch: %w", err)
	}

	var resp WallGetResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal posts batch: %w", err)
	}

	return &resp, nil
}

// FetchPostsExecute fetches multiple batches of posts using VK's execute method.
// Each execute call contains up to batchSize wall.get calls (max 25), each returning up to 100 posts.
// Returns the flattened list of posts.
func (c *Client) FetchPostsExecute(ctx context.Context, ownerID int, domain string, startOffset, batchSize, totalCount int) ([]WallPost, error) {
	code := buildExecuteScript(ownerID, domain, startOffset, batchSize, totalCount)

	params := url.Values{
		"code": {code},
	}

	raw, err := c.RequestWithRetry(ctx, "execute", params)
	if err != nil {
		return nil, fmt.Errorf("execute batch: %w", err)
	}

	// execute returns an array of wall.get responses (some may be false if offset exceeded total)
	var rawResults []json.RawMessage
	if err := json.Unmarshal(raw, &rawResults); err != nil {
		return nil, fmt.Errorf("unmarshal execute response: %w", err)
	}

	var allPosts []WallPost
	for _, rawItem := range rawResults {
		// VK returns false for calls that exceed the total count
		if string(rawItem) == "false" {
			continue
		}

		var resp WallGetResponse
		if err := json.Unmarshal(rawItem, &resp); err != nil {
			c.logger.Warn("failed to unmarshal execute item, skipping", "error", err)
			continue
		}
		allPosts = append(allPosts, resp.Items...)
	}

	return allPosts, nil
}

// FetchAllPosts fetches all posts from a wall using execute batching.
// The onBatch callback is called after each execute batch with the fetched posts,
// total count, current offset, and next offset.
func (c *Client) FetchAllPosts(
	ctx context.Context,
	ownerID int,
	domain string,
	startOffset int,
	batchSize int,
	onBatch func(posts []WallPost, total int, offset int, nextOffset int) error,
) error {
	totalCount, err := c.FetchWallCount(ctx, ownerID, domain)
	if err != nil {
		return err
	}

	c.logger.Info("starting post fetch", "total_posts", totalCount, "start_offset", startOffset)

	currentBatchSize := batchSize
	offset := startOffset
	for offset < totalCount {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		batchSizeUsed := currentBatchSize
		posts, err := c.FetchPostsExecute(ctx, ownerID, domain, offset, batchSizeUsed, totalCount)
		if err != nil {
			if isAPIErrorCode(err, 13) {
				for isAPIErrorCode(err, 13) && batchSizeUsed > 1 {
					nextSize := batchSizeUsed / 2
					if nextSize < 1 {
						nextSize = 1
					}
					c.logger.Warn("execute response too big, reducing batch size",
						"offset", offset,
						"batch_size", batchSizeUsed,
						"next_batch_size", nextSize,
					)
					batchSizeUsed = nextSize
					posts, err = c.FetchPostsExecute(ctx, ownerID, domain, offset, batchSizeUsed, totalCount)
					if err == nil {
						break
					}
				}
				currentBatchSize = batchSizeUsed
				if err != nil && isAPIErrorCode(err, 13) && batchSizeUsed == 1 {
					c.logger.Warn("execute failed at minimum batch size, falling back to simple fetch",
						"error", err,
						"offset", offset,
					)
					posts, err = c.fetchPostsSimpleBatch(ctx, ownerID, domain, offset, batchSizeUsed)
				}
			}

			if err != nil {
				// Fallback to simple wall.get if execute fails
				c.logger.Warn("execute failed, falling back to simple fetch", "error", err, "offset", offset)
				posts, err = c.fetchPostsSimpleBatch(ctx, ownerID, domain, offset, batchSizeUsed)
				if err != nil {
					return fmt.Errorf("fetch posts at offset %d: %w", offset, err)
				}
			}
		}

		if len(posts) == 0 {
			c.logger.Warn("no posts returned, stopping", "offset", offset, "total", totalCount)
			break
		}

		nextOffset := offset + batchSizeUsed*100
		if err := onBatch(posts, totalCount, offset, nextOffset); err != nil {
			return fmt.Errorf("onBatch at offset %d: %w", offset, err)
		}

		offset = nextOffset
	}

	return nil
}

// fetchPostsSimpleBatch is a fallback that makes individual wall.get calls.
func (c *Client) fetchPostsSimpleBatch(ctx context.Context, ownerID int, domain string, startOffset, batchSize int) ([]WallPost, error) {
	var allPosts []WallPost
	for i := 0; i < batchSize; i++ {
		offset := startOffset + i*100

		select {
		case <-ctx.Done():
			return allPosts, ctx.Err()
		default:
		}

		resp, err := c.FetchPostsBatch(ctx, ownerID, domain, offset, 100)
		if err != nil {
			return allPosts, err
		}
		if len(resp.Items) == 0 {
			break
		}
		allPosts = append(allPosts, resp.Items...)
	}
	return allPosts, nil
}

func buildExecuteScript(ownerID int, domain string, startOffset, batchSize, totalCount int) string {
	var ownerParam string
	if domain != "" {
		ownerParam = fmt.Sprintf(`"domain":"%s"`, domain)
	} else {
		ownerParam = fmt.Sprintf(`"owner_id":%d`, ownerID)
	}

	var sb strings.Builder
	sb.WriteString("var results = [];\n")
	sb.WriteString(fmt.Sprintf("var offset = %d;\n", startOffset))
	sb.WriteString("var i = 0;\n")
	sb.WriteString(fmt.Sprintf("while (i < %d && offset < %d) {\n", batchSize, totalCount))
	sb.WriteString(fmt.Sprintf(`  results.push(API.wall.get({%s, "count": 100, "offset": offset}));`+"\n", ownerParam))
	sb.WriteString("  offset = offset + 100;\n")
	sb.WriteString("  i = i + 1;\n")
	sb.WriteString("}\n")
	sb.WriteString("return results;")

	return sb.String()
}

func setOwnerParam(params url.Values, ownerID int, domain string) {
	if domain != "" {
		params.Set("domain", domain)
	} else {
		params.Set("owner_id", strconv.Itoa(ownerID))
	}
}
