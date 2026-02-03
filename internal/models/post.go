package models

// Output is the top-level JSON output structure.
type Output struct {
	Community  string `json:"community"`
	TotalPosts int    `json:"total_posts"`
	FetchedAt  string `json:"fetched_at"`
	Posts      []Post `json:"posts"`
}

// Post represents a single parsed post for JSON output.
type Post struct {
	ID        int     `json:"id"`
	OwnerID   int     `json:"owner_id"`
	URL       string  `json:"url"`
	Text      string  `json:"text"`
	DateUnix  int64   `json:"date_unix"`
	DateHuman string  `json:"date_human"`
	PostType  string  `json:"post_type"`
	IsPinned  bool    `json:"is_pinned"`
	Photos    []Photo `json:"photos,omitempty"`
	Links     []Link  `json:"links,omitempty"`
	Likes     int     `json:"likes"`
	Reposts   int     `json:"reposts"`
	Comments  int     `json:"comments"`
	Views     int     `json:"views"`
	IsRepost  bool    `json:"is_repost"`
	RepostOf  *Post   `json:"repost_of,omitempty"`
}

// Photo represents a photo attachment in the output.
type Photo struct {
	URL       string `json:"url"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	LocalPath string `json:"local_path,omitempty"`
}

// Link represents a link attachment in the output.
type Link struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}
