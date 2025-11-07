package testutils

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// TestConfig represents the test configuration structure
type TestConfig struct {
	Test struct {
		Database struct {
			Container struct {
				Image    string `yaml:"image"`
				Port     int    `yaml:"port"`
				Database string `yaml:"database"`
				Username string `yaml:"username"`
				Password string `yaml:"password"`
			} `yaml:"container"`
			SourceDB string `yaml:"source_db"`
			TargetDB string `yaml:"target_db"`
		} `yaml:"database"`
		Data struct {
			FixturesPath string `yaml:"fixtures_path"`
			TempDir      string `yaml:"temp_dir"`
		} `yaml:"data"`
		Execution struct {
			Timeout          string `yaml:"timeout"`
			ParallelWorkers  int    `yaml:"parallel_workers"`
			CleanupOnFailure bool   `yaml:"cleanup_on_failure"`
		} `yaml:"execution"`
		Performance struct {
			LargeDatasetRows    int `yaml:"large_dataset_rows"`
			MemoryLimitMB       int `yaml:"memory_limit_mb"`
			BenchmarkIterations int `yaml:"benchmark_iterations"`
		} `yaml:"performance"`
		Integration struct {
			ContainerStartupTimeout string `yaml:"container_startup_timeout"`
			TestDataSize            string `yaml:"test_data_size"`
			EnableLogging           bool   `yaml:"enable_logging"`
		} `yaml:"integration"`
	} `yaml:"test"`
}

// LoadTestConfig loads the test configuration from the config file
func LoadTestConfig() (*TestConfig, error) {
	// Try different possible paths for the config file
	possiblePaths := []string{
		filepath.Join("tests", "config.yaml"),
		filepath.Join("..", "config.yaml"),
		"config.yaml",
	}

	var data []byte
	var err error

	for _, path := range possiblePaths {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, err
	}

	var config TestConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// GetTimeout returns the parsed timeout duration
func (c *TestConfig) GetTimeout() time.Duration {
	duration, err := time.ParseDuration(c.Test.Execution.Timeout)
	if err != nil {
		return 30 * time.Second // default fallback
	}
	return duration
}

// GetContainerStartupTimeout returns the parsed container startup timeout
func (c *TestConfig) GetContainerStartupTimeout() time.Duration {
	duration, err := time.ParseDuration(c.Test.Integration.ContainerStartupTimeout)
	if err != nil {
		return 60 * time.Second // default fallback
	}
	return duration
}
