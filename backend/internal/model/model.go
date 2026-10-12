package model

import "time"

type User struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Username     string    `json:"username" gorm:"uniqueIndex;size:32;not null"`
	DisplayName  string    `json:"display_name" gorm:"size:40;not null"`
	PasswordHash string    `json:"-" gorm:"not null"`
	CreatedAt    time.Time `json:"created_at"`
}
type Session struct {
	TokenHash string    `gorm:"primaryKey;size:64"`
	UserID    uint      `gorm:"index;not null"`
	ExpiresAt time.Time `gorm:"index;not null"`
}
type Table struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	OwnerID     uint      `json:"owner_id" gorm:"index;not null"`
	Name        string    `json:"name" gorm:"size:80;not null"`
	Description string    `json:"description" gorm:"size:300"`
	Visibility  string    `json:"visibility" gorm:"size:10;not null;default:private"`
	CreatedAt   time.Time `json:"created_at"`
}
type Member struct {
	TableID   uint      `json:"table_id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"primaryKey"`
	Role      string    `json:"role" gorm:"size:10;not null"`
	Status    string    `json:"status" gorm:"size:10;not null"`
	CreatedAt time.Time `json:"created_at"`
}
type Cat struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	TableID   uint      `json:"table_id" gorm:"index;not null"`
	Name      string    `json:"name" gorm:"size:40;not null"`
	Breed     string    `json:"breed" gorm:"size:60"`
	Sex       string    `json:"sex" gorm:"size:10"`
	Birthday  string    `json:"birthday" gorm:"size:10"`
	Color     string    `json:"color" gorm:"size:16"`
	Neutered  bool      `json:"neutered"`
	Archived  bool      `json:"archived"`
	Notes     string    `json:"notes" gorm:"size:500"`
	CreatedAt time.Time `json:"created_at"`
}
type Litter struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	TableID     uint   `json:"table_id" gorm:"index;not null"`
	Name        string `json:"name" gorm:"size:60;not null"`
	Material    string `json:"material" gorm:"size:16;not null"`
	WaitMinutes int    `json:"wait_minutes"`
	Notes       string `json:"notes" gorm:"size:300"`
	Version     int    `json:"version" gorm:"not null;default:1"`
}
type Box struct {
	ID       uint   `json:"id" gorm:"primaryKey"`
	TableID  uint   `json:"table_id" gorm:"index;not null"`
	Name     string `json:"name" gorm:"size:60;not null"`
	Location string `json:"location" gorm:"size:80"`
	LitterID uint   `json:"litter_id" gorm:"not null"`
	CatIDs   []uint `json:"cat_ids" gorm:"serializer:json;type:jsonb"`
	Archived bool   `json:"archived"`
}
type Record struct {
	ID                   uint       `json:"id" gorm:"primaryKey"`
	TableID              uint       `json:"table_id" gorm:"index;not null"`
	CreatorID            uint       `json:"creator_id"`
	Kind                 string     `json:"kind" gorm:"size:10;not null"`
	Mode                 string     `json:"mode" gorm:"size:12;not null;default:observed"`
	BatchID              string     `json:"batch_id" gorm:"size:32;index"`
	CatID                *uint      `json:"cat_id" gorm:"index"`
	BoxID                uint       `json:"box_id" gorm:"index;not null"`
	OccurredAt           time.Time  `json:"occurred_at" gorm:"index;not null"`
	FormedAt             *time.Time `json:"formed_at"`
	LastCleanedAt        *time.Time `json:"last_cleaned_at"`
	LastCleaningComplete bool       `json:"last_cleaning_complete"`
	CollectionComplete   bool       `json:"collection_complete"`
	Integrity            string     `json:"integrity" gorm:"size:16;not null;default:unknown"`
	EventID              string     `json:"event_id" gorm:"size:64;index"`
	IdentityMethod       string     `json:"identity_method" gorm:"size:16;not null;default:unknown"`
	BaselineVersion      int        `json:"baseline_version" gorm:"not null;default:1"`
	LengthCM             *float64   `json:"length_cm"`
	WidthCM              *float64   `json:"width_cm"`
	ThicknessCM          *float64   `json:"thickness_cm"`
	LitterDepthCM        *float64   `json:"litter_depth_cm"`
	Count                int        `json:"count"`
	Grams                *float64   `json:"grams"`
	DiameterCM           *float64   `json:"diameter_cm"`
	Looseness            *int       `json:"looseness"`
	Moisture             *int       `json:"moisture"`
	Shape                string     `json:"shape" gorm:"size:16"`
	Size                 *int       `json:"size"`
	BodyWeightKG         *float64   `json:"body_weight_kg"`
	Blood                bool       `json:"blood"`
	Straining            bool       `json:"straining"`
	NoOutput             bool       `json:"no_output"`
	Notes                string     `json:"notes" gorm:"size:500"`
	LitterKey            string     `json:"litter_key" gorm:"size:40;index"`
	LitterName           string     `json:"litter_name" gorm:"size:60"`
	WaitMinutes          int        `json:"wait_minutes"`
	EligibleCatIDs       []uint     `json:"eligible_cat_ids" gorm:"serializer:json;type:jsonb"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}
type Weight struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	TableID    uint      `json:"table_id" gorm:"index;not null"`
	CatID      uint      `json:"cat_id" gorm:"index;not null"`
	CreatorID  uint      `json:"creator_id"`
	KG         float64   `json:"kg"`
	OccurredAt time.Time `json:"occurred_at" gorm:"index;not null"`
	Notes      string    `json:"notes" gorm:"size:300"`
	CreatedAt  time.Time `json:"created_at"`
}
