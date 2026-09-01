package app

import "time"

type Profile struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	DefaultLanguage string    `json:"default_language"`
	Description     string    `json:"description"`
	CreatedAt       time.Time `json:"created_at"`
}
type Account struct {
	ID        string    `json:"id"`
	ProfileID string    `json:"profile_id"`
	Platform  string    `json:"platform"`
	Name      string    `json:"name"`
	RemoteID  string    `json:"remote_id"`
	BaseURL   string    `json:"base_url"`
	TokenEnv  string    `json:"token_env"`
	CreatedAt time.Time `json:"created_at"`
}
type Topic struct {
	ID        string    `json:"id"`
	ProfileID string    `json:"profile_id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type BaseDraft struct {
	ID        string    `json:"id"`
	TopicID   string    `json:"topic_id"`
	Text      string    `json:"text"`
	Format    string    `json:"format"`
	Language  string    `json:"language"`
	Revision  int       `json:"revision"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Variant struct {
	ID             string    `json:"id"`
	TopicID        string    `json:"topic_id"`
	AccountID      string    `json:"account_id"`
	Text           string    `json:"text"`
	Format         string    `json:"format"`
	Status         string    `json:"status"`
	Relation       string    `json:"relation"`
	SourceRevision string    `json:"source_revision"`
	Visibility     string    `json:"visibility"`
	SpoilerText    string    `json:"spoiler_text"`
	LocalOnly      bool      `json:"local_only"`
	Outdated       bool      `json:"outdated"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
type Media struct {
	ID        string    `json:"id"`
	TopicID   string    `json:"topic_id"`
	VariantID string    `json:"variant_id"`
	Filename  string    `json:"filename"`
	MIME      string    `json:"mime"`
	Path      string    `json:"-"`
	AltText   string    `json:"alt_text"`
	Size      int64     `json:"size"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	CreatedAt time.Time `json:"created_at"`
}
type Publication struct {
	ID             string    `json:"id"`
	VariantID      string    `json:"variant_id"`
	AccountID      string    `json:"account_id"`
	Status         string    `json:"status"`
	RemoteID       string    `json:"remote_id"`
	RemoteURL      string    `json:"remote_url"`
	RemoteCID      string    `json:"remote_cid"`
	ErrorCategory  string    `json:"error_category"`
	ErrorMessage   string    `json:"error_message"`
	IdempotencyKey string    `json:"idempotency_key"`
	Source         string    `json:"source"`
	PublishedAt    time.Time `json:"published_at"`
	CreatedAt      time.Time `json:"created_at"`
}
type ExternalPost struct {
	ID           string    `json:"id"`
	TopicID      string    `json:"topic_id"`
	AccountID    string    `json:"account_id"`
	Platform     string    `json:"platform"`
	RemoteID     string    `json:"remote_id"`
	URL          string    `json:"url"`
	TextSnapshot string    `json:"text_snapshot"`
	CapturedBy   string    `json:"captured_by"`
	CreatedAt    time.Time `json:"created_at"`
}
