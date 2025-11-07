package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type DatabaseConfig struct {
	Type         string `yaml:"type"`
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	Database     string `yaml:"database"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	SSLMode      string `yaml:"sslmode"`
	URI          string `yaml:"uri"`
	AuthDatabase string `yaml:"auth_database"`
}

type Config struct {
	Database DatabaseConfig `yaml:"database"`
}

func LoadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	config.Database.Type = normalizeDatabaseType(config.Database.Type)

	if config.Database.Type == "postgres" && config.Database.SSLMode == "" {
		config.Database.SSLMode = "disable"
	}
	if config.Database.Type == "mongo" && config.Database.Port == 0 {
		config.Database.Port = 27017
	}

	return &config, nil
}

func (c *Config) GetConnectionString() string {
	if c.Database.Type != "" && c.Database.Type != "postgres" {
		return ""
	}

	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Database.Host,
		c.Database.Port,
		c.Database.Username,
		c.Database.Password,
		c.Database.Database,
		c.Database.SSLMode,
	)
}

func (c *Config) GetMongoURI() string {
	if c.Database.URI != "" {
		return c.Database.URI
	}

	host := c.Database.Host
	if host == "" {
		host = "localhost"
	}
	port := c.Database.Port
	if port == 0 {
		port = 27017
	}

	var credentials string
	if c.Database.Username != "" {
		credentials = url.QueryEscape(c.Database.Username)
		if c.Database.Password != "" {
			credentials = fmt.Sprintf("%s:%s", credentials, url.QueryEscape(c.Database.Password))
		}
		credentials += "@"
	}

	targetDatabase := strings.TrimSpace(c.Database.Database)
	if targetDatabase != "" {
		targetDatabase = "/" + targetDatabase
	}

	uri := fmt.Sprintf("mongodb://%s%s:%d%s", credentials, host, port, targetDatabase)

	if c.Database.AuthDatabase != "" {
		uri = fmt.Sprintf("%s?authSource=%s", uri, url.QueryEscape(c.Database.AuthDatabase))
	}

	return uri
}

func normalizeDatabaseType(dbType string) string {
	dbType = strings.ToLower(strings.TrimSpace(dbType))
	if dbType == "" {
		return "postgres"
	}

	switch dbType {
	case "postgres", "postgresql":
		return "postgres"
	case "mongo", "mongodb":
		return "mongo"
	default:
		return dbType
	}
}
