package parser

import (
	"fmt"
	"time"

	"vk-post-parser/internal/models"
	"vk-post-parser/internal/vkapi"
)

// Photo size type priority from largest to smallest.
var sizePriority = []string{"w", "z", "y", "x", "r", "q", "p", "o", "m", "s"}

// ParsePosts converts VK API posts into domain models.
func ParsePosts(vkPosts []vkapi.WallPost) []models.Post {
	posts := make([]models.Post, 0, len(vkPosts))
	for _, vp := range vkPosts {
		posts = append(posts, parsePost(vp))
	}
	return posts
}

func parsePost(vp vkapi.WallPost) models.Post {
	p := models.Post{
		ID:        vp.ID,
		OwnerID:   vp.OwnerID,
		URL:       postURL(vp.OwnerID, vp.ID),
		Text:      vp.Text,
		DateUnix:  vp.Date,
		DateHuman: humanDate(vp.Date),
		PostType:  vp.PostType,
		IsPinned:  vp.IsPinned == 1,
		Likes:     vp.Likes.Count,
		Reposts:   vp.Reposts.Count,
		Comments:  vp.Comments.Count,
	}

	if vp.Views != nil {
		p.Views = vp.Views.Count
	}

	// Parse attachments
	for _, att := range vp.Attachments {
		switch att.Type {
		case "photo":
			if att.Photo != nil {
				photo := extractPhoto(att.Photo)
				if photo.URL != "" {
					p.Photos = append(p.Photos, photo)
				}
			}
		case "link":
			if att.Link != nil {
				p.Links = append(p.Links, models.Link{
					URL:         att.Link.URL,
					Title:       att.Link.Title,
					Description: att.Link.Description,
				})
			}
		}
	}

	// Handle reposts
	if len(vp.CopyHistory) > 0 {
		p.IsRepost = true
		repost := parsePost(vp.CopyHistory[0])
		p.RepostOf = &repost
	}

	return p
}

func extractPhoto(photo *vkapi.PhotoAttachment) models.Photo {
	// Try modern sizes array first
	if len(photo.Sizes) > 0 {
		return bestFromSizes(photo.Sizes)
	}

	// Fallback to deprecated photo_XXX fields for old posts
	return bestFromLegacy(photo)
}

func bestFromSizes(sizes []vkapi.PhotoSize) models.Photo {
	sizeMap := make(map[string]vkapi.PhotoSize, len(sizes))
	for _, s := range sizes {
		sizeMap[s.Type] = s
	}

	for _, t := range sizePriority {
		if s, ok := sizeMap[t]; ok {
			return models.Photo{
				URL:    s.URL,
				Width:  s.Width,
				Height: s.Height,
			}
		}
	}

	// Last resort: pick the largest by area
	var best vkapi.PhotoSize
	bestArea := 0
	for _, s := range sizes {
		area := s.Width * s.Height
		if area > bestArea {
			bestArea = area
			best = s
		}
	}

	return models.Photo{
		URL:    best.URL,
		Width:  best.Width,
		Height: best.Height,
	}
}

func bestFromLegacy(photo *vkapi.PhotoAttachment) models.Photo {
	// Check from largest to smallest
	candidates := []string{
		photo.Photo2560,
		photo.Photo1280,
		photo.Photo807,
		photo.Photo604,
		photo.Photo130,
		photo.Photo75,
	}

	for _, url := range candidates {
		if url != "" {
			return models.Photo{
				URL:    url,
				Width:  photo.Width,
				Height: photo.Height,
			}
		}
	}

	return models.Photo{}
}

func postURL(ownerID, postID int) string {
	return fmt.Sprintf("https://vk.com/wall%d_%d", ownerID, postID)
}

func humanDate(unix int64) string {
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}
