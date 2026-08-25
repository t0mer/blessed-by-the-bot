package store

import "time"

// Event types, relations and genders mirror the CHECK constraints in
// migrations/0001_init.sql. Keep them in sync.
const (
	EventBirthday    = "birthday"
	EventWedding     = "wedding"
	EventAnniversary = "anniversary"
	EventCustom      = "custom"

	GenderMale   = "male"
	GenderFemale = "female"
	GenderOther  = "other"

	RelationFriend      = "friend"
	RelationCloseFriend = "close_friend"
	RelationFamily      = "family"
	RelationCoworker    = "coworker"

	KindScheduled = "scheduled"
	KindGroupEcho = "group_echo"

	StatusSent   = "sent"
	StatusFailed = "failed"
)

// Contact is a person with a recurring event.
type Contact struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Phone      string    `json:"phone"`
	EventDate  string    `json:"event_date"` // YYYY-MM-DD
	EventType  string    `json:"event_type"`
	Language   string    `json:"language"`
	Relation   string    `json:"relation"`
	Importance int       `json:"importance"`
	Gender     string    `json:"gender"`
	SendTime   *string   `json:"send_time"` // HH:MM override, nil = use the general setting
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Blessing is a message template. Gender and Relation nil mean "any".
type Blessing struct {
	ID        int64     `json:"id"`
	EventType string    `json:"event_type"`
	Language  string    `json:"language"`
	Gender    *string   `json:"gender"`
	Relation  *string   `json:"relation"`
	Text      string    `json:"text"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Group is a watched WhatsApp group.
type Group struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	ChatID    string    `json:"chat_id"`
	Language  string    `json:"language"`
	Threshold *int      `json:"threshold"` // nil = use the global default
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WishEvent records one wish-like message seen in a watched group.
type WishEvent struct {
	ID        int64     `json:"id"`
	GroupID   int64     `json:"group_id"`
	SenderID  string    `json:"sender_id"`
	MessageID string    `json:"message_id"`
	Matched   string    `json:"matched"`
	CreatedAt time.Time `json:"created_at"`
}

// WishPattern is an editable wish-detection rule. A pattern wrapped in slashes
// is treated as a regex by the echo service; anything else is a substring.
type WishPattern struct {
	ID       int64  `json:"id"`
	Language string `json:"language"`
	Pattern  string `json:"pattern"`
	Enabled  bool   `json:"enabled"`
}

// SendLogEntry is one send attempt, successful or not.
type SendLogEntry struct {
	ID         int64     `json:"id"`
	Kind       string    `json:"kind"`
	ContactID  *int64    `json:"contact_id"`
	GroupID    *int64    `json:"group_id"`
	BlessingID *int64    `json:"blessing_id"`
	Provider   string    `json:"provider"`
	ChatID     string    `json:"chat_id"`
	Status     string    `json:"status"`
	Error      *string   `json:"error"`
	EventYear  *int      `json:"event_year"`
	SentAt     time.Time `json:"sent_at"`
}
