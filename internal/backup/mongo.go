package backup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kadirbelkuyu/DBRTS/internal/config"
	"github.com/kadirbelkuyu/DBRTS/pkg/logger"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type mongoService struct {
	cfg    *config.Config
	log    *logger.Logger
	client *mongo.Client
}

func newMongoService(cfg *config.Config, log *logger.Logger) *mongoService {
	return &mongoService{
		cfg: cfg,
		log: log,
	}
}

func (s *mongoService) Connect() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(s.cfg.GetMongoURI()))
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	s.client = client
	return nil
}

func (s *mongoService) Close() error {
	if s.client == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.client.Disconnect(ctx)
}

func (s *mongoService) ListDatabases() ([]DatabaseInfo, error) {
	if s.client == nil {
		if err := s.Connect(); err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := s.client.ListDatabases(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("failed to list MongoDB databases: %w", err)
	}

	databases := make([]DatabaseInfo, 0, len(result.Databases))
	for _, db := range result.Databases {
		info := DatabaseInfo{
			Name: db.Name,
			Type: "mongo",
		}

		if db.SizeOnDisk > 0 {
			sizeMB := float64(db.SizeOnDisk) / (1024 * 1024)
			info.Size = fmt.Sprintf("%.2f MB", sizeMB)
		} else {
			info.Size = "0 MB"
		}

		collections, err := s.countCollections(db.Name)
		if err == nil {
			info.Collections = collections
		}

		databases = append(databases, info)
	}

	return databases, nil
}

func (s *mongoService) CreateBackup(databaseName string, options BackupOptions) (*BackupMetadata, error) {
	start := time.Now()

	outputPath, err := s.ensureOutputPath(databaseName, options)
	if err != nil {
		return nil, err
	}

	args := s.buildDumpArgs(databaseName, outputPath, options)
	if err := s.runCommand("mongodump", args, options.Verbose); err != nil {
		return nil, err
	}

	return buildBackupMetadata(outputPath, start)
}

func (s *mongoService) RestoreBackup(options RestoreOptions) error {
	if _, err := os.Stat(options.BackupPath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	args := []string{
		fmt.Sprintf("--uri=%s", s.cfg.GetMongoURI()),
		fmt.Sprintf("--archive=%s", options.BackupPath),
	}

	if options.TargetDatabase != "" {
		args = append(args, fmt.Sprintf("--nsInclude=%s.*", options.TargetDatabase))
	}

	if options.CleanFirst {
		args = append(args, "--drop")
	}

	if options.Verbose {
		args = append(args, "--verbose")
	}

	if options.ExitOnError {
		args = append(args, "--stopOnError")
	}

	return s.runCommand("mongorestore", args, options.Verbose)
}

func (s *mongoService) ensureOutputPath(databaseName string, options BackupOptions) (string, error) {
	outputPath := options.OutputPath
	if outputPath == "" {
		if err := os.MkdirAll("backup", 0o755); err != nil {
			return "", fmt.Errorf("failed to create backup directory: %w", err)
		}

		extension := ".archive"
		if options.Compression > 0 {
			extension = ".archive.gz"
		}

		fileName := fmt.Sprintf("%s_%s%s", databaseName, time.Now().Format("20060102_150405"), extension)
		outputPath = filepath.Join("backup", fileName)
	} else {
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return "", fmt.Errorf("failed to prepare backup directory: %w", err)
		}
	}

	return outputPath, nil
}

func (s *mongoService) buildDumpArgs(databaseName, outputPath string, options BackupOptions) []string {
	args := []string{
		fmt.Sprintf("--uri=%s", s.cfg.GetMongoURI()),
		fmt.Sprintf("--archive=%s", outputPath),
	}

	if databaseName != "" {
		args = append(args, fmt.Sprintf("--db=%s", databaseName))
	}

	if options.Compression > 0 {
		args = append(args, "--gzip")
	}

	if options.Verbose {
		args = append(args, "--verbose")
	}

	return args
}

func (s *mongoService) runCommand(name string, args []string, verbose bool) error {
	cmd := exec.Command(name, args...)
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		writer := s.log.Writer()
		defer writer.Close()
		cmd.Stdout = writer
		cmd.Stderr = writer
	}

	s.log.Debugf("executing %s %s", name, strings.Join(args, " "))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", name, err)
	}

	return nil
}

func (s *mongoService) countCollections(databaseName string) (int, error) {
	if databaseName == "" {
		return 0, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	collections, err := s.client.Database(databaseName).ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return 0, err
	}

	return len(collections), nil
}
