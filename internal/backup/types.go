package backup

import "time"

type DatabaseInfo struct {
	Name        string
	Owner       string
	Encoding    string
	Size        string
	Collections int
	Type        string
}

type BackupOptions struct {
	Format      string
	Compression int
	SchemaOnly  bool
	DataOnly    bool
	OutputPath  string
	Verbose     bool
}

type RestoreOptions struct {
	BackupPath     string
	TargetDatabase string
	CreateDatabase bool
	CleanFirst     bool
	Verbose        bool
	ExitOnError    bool
}

type BackupMetadata struct {
	BackupSize  int64
	Checksum    string
	Location    string
	StartedAt   time.Time
	CompletedAt time.Time
}
