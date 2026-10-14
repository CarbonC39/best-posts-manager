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
	ID        string `json:"id"`
	ProfileID string `json:"profile_id"`
	// ProfileIDs is the set of workspaces that may use this connection. ProfileID
	// remains the original owner for backwards-compatible local data.
	ProfileIDs []string `json:"profile_ids,omitempty"`
	Platform   string   `json:"platform"`
	Name       string   `json:"name"`
	RemoteID   string   `json:"remote_id"`
	BaseURL    string   `json:"base_url"`
	TokenEnv   string   `json:"token_env"`
	// CredentialRef is an internal pointer into the local credential store. It
	// must never cross the HTTP/CLI JSON boundary.
	CredentialRef string    `json:"-"`
	Disabled      bool      `json:"disabled"`
	CreatedAt     time.Time `json:"created_at"`
}
type Topic struct {
	ID        string    `json:"id"`
	ProfileID string    `json:"profile_id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TopicSummary is the lightweight operational view used by topic lists. Counts
// are derived from persisted variants and publications rather than UI guesses.
type TopicSummary struct {
	Topic
	TargetCount      int      `json:"target_count"`
	PublishedCount   int      `json:"published_count"`
	NeedsActionCount int      `json:"needs_action_count"`
	InProgressCount  int      `json:"in_progress_count"`
	Platforms        []string `json:"platforms"`
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
	ID                string    `json:"id"`
	TopicID           string    `json:"topic_id"`
	AccountID         string    `json:"account_id"`
	Text              string    `json:"text"`
	Format            string    `json:"format"`
	Status            string    `json:"status"`
	Relation          string    `json:"relation"`
	SourceRevision    string    `json:"source_revision"`
	Visibility        string    `json:"visibility"`
	SpoilerText       string    `json:"spoiler_text"`
	LocalOnly         bool      `json:"local_only"`
	Outdated          bool      `json:"outdated"`
	Published         bool      `json:"published"`
	PublicationLocked bool      `json:"publication_locked"`
	PublicationStatus string    `json:"publication_status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
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

const (
	PublicationPending           = "pending"
	PublicationPublishing        = "publishing"
	PublicationNeedsConfirmation = "needs_confirmation"
	PublicationFailed            = "failed"
	PublicationPublished         = "published"
	PublicationCancelled         = "cancelled"
)

// ValidPublicationStatus and CanTransitionPublication keep publication state
// rules in one place while preserving the string-valued JSON contract.
func ValidPublicationStatus(status string) bool {
	switch status {
	case PublicationPending, PublicationPublishing, PublicationNeedsConfirmation,
		PublicationFailed, PublicationPublished, PublicationCancelled:
		return true
	default:
		return false
	}
}

func CanTransitionPublication(from, to string) bool {
	if from == to {
		return ValidPublicationStatus(to)
	}
	switch from {
	case PublicationPending:
		return to == PublicationPublishing || to == PublicationFailed || to == PublicationNeedsConfirmation || to == PublicationCancelled
	case PublicationPublishing:
		return to == PublicationPublished || to == PublicationFailed || to == PublicationNeedsConfirmation
	case PublicationNeedsConfirmation:
		return to == PublicationPublished || to == PublicationFailed || to == PublicationCancelled
	case PublicationFailed:
		return to == PublicationPublishing
	default:
		return false
	}
}

// BridgeRequestRecord stores a protocol payload without coupling app storage
// to the bridge package.
type BridgeRequestRecord struct {
	ID        string
	PairingID string
	Status    string
	ExpiresAt time.Time
	Payload   []byte
}

// PublicationSummary keeps the immutable publication record while adding the
// topic context needed to identify and resolve it in the publishing center.
type PublicationSummary struct {
	Publication
	TopicID    string `json:"topic_id"`
	TopicTitle string `json:"topic_title"`
	Text       string `json:"text"`
}

type PublicationStatusSummary struct {
	ActionCount int `json:"action_count"`
	FailedCount int `json:"failed_count"`
}
type ExternalPost struct {
	ID               string          `json:"id"`
	TopicID          string          `json:"topic_id"`
	AccountID        string          `json:"account_id"`
	Platform         string          `json:"platform"`
	RemoteID         string          `json:"remote_id"`
	URL              string          `json:"url"`
	TextSnapshot     string          `json:"text_snapshot"`
	MediaSnapshot    []ExternalMedia `json:"media_snapshot"`
	CapturedBy       string          `json:"captured_by"`
	Incomplete       bool            `json:"incomplete"`
	IncompleteReason string          `json:"incomplete_reason"`
	CreatedAt        time.Time       `json:"created_at"`
}
type ExternalMedia struct {
	Filename string `json:"filename"`
	MIME     string `json:"mime"`
	URL      string `json:"url"`
	AltText  string `json:"alt_text"`
	Size     int64  `json:"size"`
}

// TranslationConfig contains provider configuration, never a provider secret.
// TokenEnv/KeyEnv are names of environment variables read only at operation time.
type TranslationConfig struct {
	ID            string    `json:"id"`
	Provider      string    `json:"provider"`
	BaseURL       string    `json:"base_url"`
	ProjectID     string    `json:"project_id"`
	Location      string    `json:"location"`
	TokenEnv      string    `json:"token_env"`
	KeyEnv        string    `json:"key_env"`
	CredentialRef string    `json:"-"`
	Model         string    `json:"model"`
	CreatedAt     time.Time `json:"created_at"`
}

// BridgePairing stores only the server-side hash and lifecycle metadata. The
// bearer token and page URL are never persisted.
type BridgePairing struct {
	ID             string    `json:"id"`
	Platform       string    `json:"platform"`
	TokenHash      string    `json:"-"`
	TokenExpiresAt time.Time `json:"token_expires_at"`
	Revoked        bool      `json:"revoked"`
	CreatedAt      time.Time `json:"created_at"`
}

type Translation struct {
	ID              string    `json:"id"`
	SourceVariantID string    `json:"source_variant_id"`
	TargetAccountID string    `json:"target_account_id"`
	TargetVariantID string    `json:"target_variant_id"`
	SourceLanguage  string    `json:"source_language"`
	TargetLanguage  string    `json:"target_language"`
	Provider        string    `json:"provider"`
	Status          string    `json:"status"`
	ErrorCategory   string    `json:"error_category"`
	ErrorMessage    string    `json:"error_message"`
	CreatedAt       time.Time `json:"created_at"`
}
