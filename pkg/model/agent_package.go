package model

import (
	"encoding/json"
	"github.com/google/uuid"
	"time"
)

type AgentPackage struct {
	ID              uuid.UUID   `gorm:"type:text;primaryKey" json:"id"`
	Slug            string      `gorm:"type:varchar(128);not null;uniqueIndex:ux_agent_package_slug" json:"slug"`
	DisplayName     string      `gorm:"type:varchar(256);not null" json:"displayName"`
	Kind            string      `gorm:"type:varchar(16);not null;index" json:"kind"`
	Summary         string      `gorm:"type:text" json:"summary"`
	Tags            StringArray `gorm:"type:text;not null;default:'[]'" json:"tags"`
	OwnerID         uuid.UUID   `gorm:"type:text;not null;index" json:"ownerId"`
	LatestVersionID *uuid.UUID  `gorm:"type:text" json:"-"`
	VersionsCount   int         `json:"versionsCount"`
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `gorm:"index" json:"updatedAt"`
	SoftDeletedAt   *time.Time  `gorm:"index" json:"-"`
}

func (AgentPackage) TableName() string { return "agent_packages" }

type AgentPackageVersion struct {
	ID             uuid.UUID       `gorm:"type:text;primaryKey" json:"id"`
	AgentPackageID uuid.UUID       `gorm:"type:text;not null;uniqueIndex:ux_agent_package_version,priority:1;index" json:"-"`
	Version        string          `gorm:"type:varchar(64);not null;uniqueIndex:ux_agent_package_version,priority:2" json:"version"`
	Instructions   string          `gorm:"type:text" json:"instructions"`
	Changelog      string          `gorm:"type:text" json:"changelog"`
	ArchiveKey     string          `gorm:"type:text;not null" json:"-"`
	ArchiveSHA256  string          `gorm:"type:varchar(64);not null" json:"archiveSHA256"`
	ArchiveSize    int64           `json:"archiveSize"`
	Files          json.RawMessage `gorm:"type:text;not null;default:'[]'" json:"files"`
	CreatedBy      uuid.UUID       `gorm:"type:text;not null" json:"createdBy"`
	CreatedAt      time.Time       `json:"createdAt"`
}

func (AgentPackageVersion) TableName() string { return "agent_package_versions" }

type AgentPackageWithOwner struct {
	AgentPackage
	OwnerHandle string `json:"ownerHandle"`
}
type AgentPackageSummary struct {
	ID            uuid.UUID   `json:"id"`
	Slug          string      `json:"slug"`
	DisplayName   string      `json:"displayName"`
	Kind          string      `json:"kind"`
	Summary       string      `json:"summary"`
	Tags          StringArray `gorm:"type:text" json:"tags"`
	OwnerID       uuid.UUID   `json:"ownerId"`
	OwnerHandle   string      `json:"ownerHandle"`
	LatestVersion string      `json:"latestVersion"`
	VersionsCount int         `json:"versionsCount"`
	CreatedAt     time.Time   `json:"createdAt"`
	UpdatedAt     time.Time   `json:"updatedAt"`
}

func (a *AgentPackageWithOwner) ToSummary(latest string) AgentPackageSummary {
	return AgentPackageSummary{a.ID, a.Slug, a.DisplayName, a.Kind, a.Summary, a.Tags, a.OwnerID, a.OwnerHandle, latest, a.VersionsCount, a.CreatedAt, a.UpdatedAt}
}

type AgentVersionSummary struct {
	ID        uuid.UUID `json:"id"`
	Version   string    `json:"version"`
	Changelog string    `json:"changelog"`
	CreatedAt time.Time `json:"createdAt"`
}
