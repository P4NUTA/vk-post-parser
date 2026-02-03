package vkapi

import "encoding/json"

// APIResponse is the top-level wrapper for all VK API responses.
type APIResponse struct {
	Response json.RawMessage `json:"response"`
	Error    *APIError       `json:"error,omitempty"`
}

// APIError represents a VK API error.
type APIError struct {
	ErrorCode int    `json:"error_code"`
	ErrorMsg  string `json:"error_msg"`
}

func (e *APIError) Error() string {
	return e.ErrorMsg
}

// WallGetResponse is the response for wall.get method.
type WallGetResponse struct {
	Count int        `json:"count"`
	Items []WallPost `json:"items"`
}

// WallPost represents a single wall post from VK API.
type WallPost struct {
	ID          int          `json:"id"`
	OwnerID     int          `json:"owner_id"`
	FromID      int          `json:"from_id"`
	Date        int64        `json:"date"`
	Text        string       `json:"text"`
	PostType    string       `json:"post_type"`
	IsPinned    int          `json:"is_pinned,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Comments    CountObj     `json:"comments"`
	Likes       CountObj     `json:"likes"`
	Reposts     CountObj     `json:"reposts"`
	Views       *CountObj    `json:"views,omitempty"`
	MarkedAsAds int          `json:"marked_as_ads,omitempty"`
	CopyHistory []WallPost   `json:"copy_history,omitempty"`
}

// CountObj is a generic counter object used for likes, comments, reposts, views.
type CountObj struct {
	Count int `json:"count"`
}

// Attachment represents a VK post attachment.
type Attachment struct {
	Type  string           `json:"type"`
	Photo *PhotoAttachment `json:"photo,omitempty"`
	Link  *LinkAttachment  `json:"link,omitempty"`
	Video *VideoAttachment `json:"video,omitempty"`
	Doc   *DocAttachment   `json:"doc,omitempty"`
}

// PhotoAttachment represents a photo in VK API.
type PhotoAttachment struct {
	ID      int         `json:"id"`
	OwnerID int         `json:"owner_id"`
	Sizes   []PhotoSize `json:"sizes,omitempty"`
	Text    string      `json:"text"`
	Date    int64       `json:"date"`
	// Deprecated fields for old posts that lack sizes array
	Photo75   string `json:"photo_75,omitempty"`
	Photo130  string `json:"photo_130,omitempty"`
	Photo604  string `json:"photo_604,omitempty"`
	Photo807  string `json:"photo_807,omitempty"`
	Photo1280 string `json:"photo_1280,omitempty"`
	Photo2560 string `json:"photo_2560,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}

// PhotoSize represents a single photo size variant.
type PhotoSize struct {
	Type   string `json:"type"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// LinkAttachment represents a link in VK API.
type LinkAttachment struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Caption     string `json:"caption"`
	Description string `json:"description"`
}

// VideoAttachment represents a video in VK API.
type VideoAttachment struct {
	ID          int    `json:"id"`
	OwnerID     int    `json:"owner_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Duration    int    `json:"duration"`
}

// DocAttachment represents a document in VK API.
type DocAttachment struct {
	ID      int    `json:"id"`
	OwnerID int    `json:"owner_id"`
	Title   string `json:"title"`
	Size    int    `json:"size"`
	Ext     string `json:"ext"`
	URL     string `json:"url"`
}
