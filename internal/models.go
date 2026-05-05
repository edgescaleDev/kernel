package internal

import (
	"time"
)

// SchemaMigration tracks applied migrations per module.
type SchemaMigration struct {
	ModuleID  string    `gorm:"primaryKey;column:module_id"`
	Version   int       `gorm:"primaryKey;column:version"`
	Filename  string    `gorm:"column:filename;not null"`
	Checksum  string    `gorm:"column:checksum;not null"`
	AppliedAt time.Time `gorm:"column:applied_at;autoCreateTime"`
}

func (SchemaMigration) TableName() string {
	return "schema_migrations"
}

// ModuleRecord maps to the public.module_registry table.
type ModuleRecord struct {
	ID          string   `gorm:"primaryKey;column:id"`
	Name        string   `gorm:"column:name;not null"`
	Version     string   `gorm:"column:version;not null"`
	Type        string   `gorm:"column:type;not null"`
	SchemaName  string   `gorm:"column:schema_name;not null"`
	Description string   `gorm:"column:description"`
	DependsOn   []string `gorm:"column:depends_on;type:jsonb;serializer:json"`
}

func (ModuleRecord) TableName() string {
	return "module_registry"
}

// ModuleActivation maps to the public.module_activations table.
type ModuleActivation struct {
	ModuleID    string    `gorm:"primaryKey;column:module_id"`
	TenantID    string    `gorm:"primaryKey;column:tenant_id;type:uuid"`
	Active      bool      `gorm:"column:active;default:true"`
	ActivatedBy string    `gorm:"column:activated_by;type:uuid"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (ModuleActivation) TableName() string {
	return "module_activations"
}

// CronExecution records a single cron job execution for audit and admin UI.
type CronExecution struct {
	ID           string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid();column:id"`
	CronID       string     `gorm:"column:cron_id;not null"`
	InstanceID   string     `gorm:"column:instance_id;not null"`
	StartedAt    time.Time  `gorm:"column:started_at;not null"`
	FinishedAt   *time.Time `gorm:"column:finished_at"`
	Status       string     `gorm:"column:status;not null;default:running"`
	ErrorMessage *string    `gorm:"column:error_message"`
	Attempt      int        `gorm:"column:attempt;not null;default:1"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime"`
}

func (CronExecution) TableName() string {
	return "kernel_cron_executions"
}
