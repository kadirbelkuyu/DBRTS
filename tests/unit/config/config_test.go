package config_test

import (
	"embed"
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/kadirbelkuyu/DBRTS/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/*.yaml
var configSamples embed.FS

func writeSample(t *testing.T, name string) string {
	t.Helper()

	data, err := configSamples.ReadFile(filepath.Join("testdata", name))
	require.NoErrorf(t, err, "failed to read embedded sample %s", name)

	dir := t.TempDir()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, data, 0o644))

	return path
}

func TestLoadPostgresConfigDefaults(t *testing.T) {
	path := writeSample(t, "postgres.yaml")

	cfg, err := appconfig.LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "postgres", cfg.Database.Type, "default database type should be postgres")
	assert.Equal(t, "disable", cfg.Database.SSLMode, "SSL should default to disable for postgres")

	conn := cfg.GetConnectionString()
	assert.Contains(t, conn, "host=localhost")
	assert.Contains(t, conn, "port=5432")
	assert.Contains(t, conn, "user=sample")
	assert.Contains(t, conn, "dbname=sampledb")
}

func TestLoadMongoConfigDefaults(t *testing.T) {
	path := writeSample(t, "mongo-host.yaml")

	cfg, err := appconfig.LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "mongo", cfg.Database.Type)
	assert.Equal(t, 27017, cfg.Database.Port, "mongo port should default to 27017 when omitted")

	uri := cfg.GetMongoURI()
	assert.Contains(t, uri, "cluster.internal:27017")
	assert.Contains(t, uri, "analytics")
}

func TestLoadMongoConfigWithURI(t *testing.T) {
	path := writeSample(t, "mongo-uri.yaml")

	cfg, err := appconfig.LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "mongo", cfg.Database.Type)
	assert.Equal(t, "mongodb+srv://user:pass@example.mongodb.net/prod?tls=true", cfg.Database.URI)
	assert.Equal(t, cfg.Database.URI, cfg.GetMongoURI(), "explicit URI should be returned as-is")
}
