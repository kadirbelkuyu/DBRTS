package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kadirbelkuyu/DBRTS/tests/testutils"
)

// TestBasicSetup verifies that the test infrastructure is properly set up
func TestBasicSetup(t *testing.T) {
	// Test that we can load the test configuration
	config, err := testutils.LoadTestConfig()
	require.NoError(t, err, "Should be able to load test configuration")
	assert.NotNil(t, config, "Config should not be nil")

	// Verify configuration values
	assert.Equal(t, "postgres:15", config.Test.Database.Container.Image)
	assert.Equal(t, 5432, config.Test.Database.Container.Port)
	assert.Equal(t, "testdb", config.Test.Database.Container.Database)
	assert.Equal(t, "testuser", config.Test.Database.Container.Username)
	assert.Equal(t, "testpass", config.Test.Database.Container.Password)

	// Test timeout parsing
	timeout := config.GetTimeout()
	assert.Greater(t, timeout.Seconds(), 0.0, "Timeout should be greater than 0")

	// Test container startup timeout parsing
	containerTimeout := config.GetContainerStartupTimeout()
	assert.Greater(t, containerTimeout.Seconds(), 0.0, "Container timeout should be greater than 0")
}
