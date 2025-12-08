package profiles_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/kadirbelkuyu/DBRTS/internal/config"
	"github.com/kadirbelkuyu/DBRTS/internal/profiles"
)

func TestManagerSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	manager := profiles.NewManager(dir)

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Type:     "postgres",
			Host:     "db.internal",
			Port:     5432,
			Database: "postgres",
		},
	}

	profile, err := manager.Save("Prod DB", cfg)
	require.NoError(t, err)
	require.Equal(t, "postgres", profile.Type)
	require.FileExists(t, profile.Path)

	loaded, err := manager.Load(profile.Name)
	require.NoError(t, err)
	require.Equal(t, cfg.Database.Host, loaded.Database.Host)
	require.Equal(t, cfg.Database.Type, loaded.Database.Type)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestManagerListFiltersByType(t *testing.T) {
	dir := t.TempDir()
	manager := profiles.NewManager(dir)

	writeConfig(t, dir, "alpha-postgres.yaml", "postgres")
	writeConfig(t, dir, "beta-mongo.yaml", "mongo")

	all, err := manager.List("")
	require.NoError(t, err)
	require.Len(t, all, 2)

	postgresOnly, err := manager.List("postgres")
	require.NoError(t, err)
	require.Len(t, postgresOnly, 1)
	require.Equal(t, "postgres", postgresOnly[0].Type)

	mongoOnly, err := manager.List("mongo")
	require.NoError(t, err)
	require.Len(t, mongoOnly, 1)
	require.Equal(t, "mongo", mongoOnly[0].Type)
}

func writeConfig(t *testing.T, dir, name, dbType string) {
	t.Helper()

	cfg := config.Config{
		Database: config.DatabaseConfig{
			Type:     dbType,
			Host:     "localhost",
			Port:     5432,
			Database: "postgres",
		},
	}

	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	path := filepath.Join(dir, name)
	err = os.WriteFile(path, data, 0o644)
	require.NoError(t, err)
}
