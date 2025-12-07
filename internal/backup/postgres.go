package backup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kadirbelkuyu/DBRTS/internal/config"
	"github.com/kadirbelkuyu/DBRTS/internal/database"
	"github.com/kadirbelkuyu/DBRTS/pkg/logger"
)

type postgresService struct {
	cfg  *config.Config
	log  *logger.Logger
	conn *database.Connection
}

func newPostgresService(cfg *config.Config, log *logger.Logger) *postgresService {
	return &postgresService{
		cfg: cfg,
		log: log,
	}
}

func (s *postgresService) Connect() error {
	conn, err := database.NewConnection(s.cfg)
	if err != nil {
		return err
	}
	s.conn = conn
	return nil
}

func (s *postgresService) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

func (s *postgresService) ListDatabases() ([]DatabaseInfo, error) {
	if s.conn == nil {
		if err := s.Connect(); err != nil {
			return nil, err
		}
	}

	const query = `
		SELECT 
			datname,
			pg_catalog.pg_get_userbyid(datdba) AS owner,
			pg_catalog.pg_encoding_to_char(encoding) AS encoding,
			pg_size_pretty(pg_database_size(datname)) AS size
		FROM pg_database
		WHERE datistemplate = false
		ORDER BY datname;
	`

	rows, err := s.conn.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query databases: %w", err)
	}
	defer rows.Close()

	var databases []DatabaseInfo
	for rows.Next() {
		var info DatabaseInfo
		if err := rows.Scan(&info.Name, &info.Owner, &info.Encoding, &info.Size); err != nil {
			return nil, fmt.Errorf("failed to read database info: %w", err)
		}
		info.Type = "postgres"
		databases = append(databases, info)
	}

	return databases, nil
}

func (s *postgresService) CreateBackup(databaseName string, options BackupOptions) (*BackupMetadata, error) {
	start := time.Now()

	outputPath, err := s.ensureOutputPath(databaseName, options)
	if err != nil {
		return nil, err
	}

	args := s.buildDumpArgs(databaseName, outputPath, options)
	if err := s.runCommand("pg_dump", args, options.Verbose); err != nil {
		return nil, err
	}

	return buildBackupMetadata(outputPath, start)
}

func (s *postgresService) RestoreBackup(options RestoreOptions) error {
	if options.TargetDatabase == "" {
		return fmt.Errorf("target database name is required")
	}

	if _, err := os.Stat(options.BackupPath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	if options.CreateDatabase {
		if err := s.createDatabase(options.TargetDatabase, options.CleanFirst); err != nil {
			return err
		}
	}

	ext := strings.ToLower(filepath.Ext(options.BackupPath))
	if ext == ".sql" {
		return s.restoreWithPSQL(options)
	}

	return s.restoreWithPgRestore(options)
}

func (s *postgresService) ensureOutputPath(databaseName string, options BackupOptions) (string, error) {
	outputPath := options.OutputPath
	if outputPath == "" {
		if err := os.MkdirAll("backup", 0o755); err != nil {
			return "", fmt.Errorf("failed to create backup directory: %w", err)
		}

		extension := s.resolveExtension(options.Format)
		fileName := fmt.Sprintf("%s_%s%s", databaseName, time.Now().Format("20060102_150405"), extension)
		outputPath = filepath.Join("backup", fileName)
	} else {
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return "", fmt.Errorf("failed to prepare backup directory: %w", err)
		}
	}

	// Directory format expects a folder that does not yet exist.
	if s.mapFormat(options.Format) == "directory" {
		if err := os.MkdirAll(outputPath, 0o755); err != nil {
			return "", fmt.Errorf("failed to prepare directory backup path: %w", err)
		}
	}

	return outputPath, nil
}

func (s *postgresService) buildDumpArgs(databaseName, outputPath string, options BackupOptions) []string {
	format := s.mapFormat(options.Format)

	args := []string{
		fmt.Sprintf("--host=%s", s.cfg.Database.Host),
		fmt.Sprintf("--port=%d", s.cfg.Database.Port),
		fmt.Sprintf("--username=%s", s.cfg.Database.Username),
		fmt.Sprintf("--dbname=%s", databaseName),
		fmt.Sprintf("--format=%s", format),
		fmt.Sprintf("--file=%s", outputPath),
	}

	if options.SchemaOnly {
		args = append(args, "--schema-only")
	}

	if options.DataOnly {
		args = append(args, "--data-only")
	}

	if options.Verbose {
		args = append(args, "--verbose")
	}

	if options.Compression > 0 && format != "plain" {
		args = append(args, fmt.Sprintf("--compress=%d", options.Compression))
	}

	return args
}

func (s *postgresService) mapFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "sql", "plain":
		return "plain"
	case "tar":
		return "tar"
	case "directory":
		return "directory"
	default:
		return "custom"
	}
}

func (s *postgresService) resolveExtension(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "sql", "plain":
		return ".sql"
	case "tar":
		return ".tar"
	case "directory":
		return ""
	default:
		return ".dump"
	}
}

func (s *postgresService) runCommand(cmdName string, args []string, verbose bool) error {
	cmd := exec.Command(cmdName, args...)
	cmd.Env = append(os.Environ(), s.postgresEnv()...)
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		writer := s.log.Writer()
		defer writer.Close()
		cmd.Stdout = writer
		cmd.Stderr = writer
	}

	s.log.Debugf("executing %s %s", cmdName, strings.Join(args, " "))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", cmdName, err)
	}
	return nil
}

func (s *postgresService) postgresEnv() []string {
	if s.cfg.Database.Password == "" {
		return nil
	}
	return []string{fmt.Sprintf("PGPASSWORD=%s", s.cfg.Database.Password)}
}

func (s *postgresService) restoreWithPgRestore(options RestoreOptions) error {
	args := []string{
		fmt.Sprintf("--host=%s", s.cfg.Database.Host),
		fmt.Sprintf("--port=%d", s.cfg.Database.Port),
		fmt.Sprintf("--username=%s", s.cfg.Database.Username),
		fmt.Sprintf("--dbname=%s", options.TargetDatabase),
		options.BackupPath,
	}

	if options.Verbose {
		args = append(args, "--verbose")
	}

	if options.CleanFirst {
		args = append(args, "--clean")
	}

	if options.ExitOnError {
		args = append(args, "--exit-on-error")
	}

	return s.runCommand("pg_restore", args, options.Verbose)
}

func (s *postgresService) restoreWithPSQL(options RestoreOptions) error {
	if options.CleanFirst {
		if err := s.recreateDatabase(options.TargetDatabase); err != nil {
			return err
		}
	}

	args := []string{
		fmt.Sprintf("--host=%s", s.cfg.Database.Host),
		fmt.Sprintf("--port=%d", s.cfg.Database.Port),
		fmt.Sprintf("--username=%s", s.cfg.Database.Username),
		fmt.Sprintf("--dbname=%s", options.TargetDatabase),
		"--single-transaction",
		"--set=ON_ERROR_STOP=1",
		"--file=" + options.BackupPath,
	}

	if options.Verbose {
		args = append(args, "--echo-errors")
	}

	return s.runCommand("psql", args, options.Verbose)
}

func (s *postgresService) createDatabase(name string, clean bool) error {
	if clean {
		if err := s.recreateDatabase(name); err != nil {
			return err
		}
		return nil
	}

	adminConn, err := s.openAdminConnection()
	if err != nil {
		return err
	}
	defer adminConn.Close()

	var exists bool
	if err := adminConn.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check database existence: %w", err)
	}

	if !exists {
		if _, err := adminConn.DB.Exec(fmt.Sprintf("CREATE DATABASE %s", quoteIdentifier(name))); err != nil {
			return fmt.Errorf("failed to create database %s: %w", name, err)
		}
	}
	return nil
}

func (s *postgresService) recreateDatabase(name string) error {
	adminConn, err := s.openAdminConnection()
	if err != nil {
		return err
	}
	defer adminConn.Close()

	if _, err := adminConn.DB.Exec("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1", name); err != nil {
		s.log.Warnf("failed to terminate active sessions: %v", err)
	}

	if _, err := adminConn.DB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", quoteIdentifier(name))); err != nil {
		return fmt.Errorf("failed to drop database %s: %w", name, err)
	}

	if _, err := adminConn.DB.Exec(fmt.Sprintf("CREATE DATABASE %s", quoteIdentifier(name))); err != nil {
		return fmt.Errorf("failed to recreate database %s: %w", name, err)
	}
	return nil
}

func (s *postgresService) openAdminConnection() (*database.Connection, error) {
	adminConfig := *s.cfg
	adminConfig.Database = s.cfg.Database
	adminConfig.Database.Database = "postgres"
	return database.NewConnection(&adminConfig)
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
