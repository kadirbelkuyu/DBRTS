package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kadirbelkuyu/DBRTS/internal/config"
	"github.com/kadirbelkuyu/DBRTS/internal/profiles"
	"github.com/kadirbelkuyu/DBRTS/internal/ui/explorer"
)

const defaultConfigDir = "configs"

type Application struct {
	reader         *bufio.Reader
	printBanner    func()
	profileManager *profiles.Manager
	service        *Service
}

func NewApplication(r io.Reader, printBanner func()) *Application {
	if r == nil {
		r = os.Stdin
	}

	var reader *bufio.Reader
	if br, ok := r.(*bufio.Reader); ok {
		reader = br
	} else {
		reader = bufio.NewReader(r)
	}

	return &Application{
		reader:         reader,
		printBanner:    printBanner,
		profileManager: profiles.NewManager(defaultConfigDir),
		service:        NewService(),
	}
}

func (a *Application) RunInteractive() error {
	if a.printBanner != nil {
		a.printBanner()
	}
	fmt.Println("Interactive mode is ready. Press Ctrl+C or choose option 6 to exit.")

	for {
		fmt.Println()
		fmt.Println("Select an operation:")
		fmt.Println("  1) Transfer data between databases")
		fmt.Println("  2) Create a backup")
		fmt.Println("  3) Restore a backup")
		fmt.Println("  4) List databases")
		fmt.Println("  5) Explore a database with the TUI")
		fmt.Println("  6) Exit")

		fmt.Print("\nChoice: ")
		choice, err := a.readLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Println()
				fmt.Println("Exiting interactive mode.")
				return nil
			}
			return err
		}

		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "1", "transfer":
			if err := a.handleTransfer(); err != nil {
				if errors.Is(err, io.EOF) {
					fmt.Println()
					fmt.Println("Exiting interactive mode.")
					return nil
				}
				fmt.Printf("Transfer failed: %v\n", err)
			}
		case "2", "backup":
			if err := a.handleBackup(); err != nil {
				if errors.Is(err, io.EOF) {
					fmt.Println()
					fmt.Println("Exiting interactive mode.")
					return nil
				}
				fmt.Printf("Backup failed: %v\n", err)
			}
		case "3", "restore":
			if err := a.handleRestore(); err != nil {
				if errors.Is(err, io.EOF) {
					fmt.Println()
					fmt.Println("Exiting interactive mode.")
					return nil
				}
				fmt.Printf("Restore failed: %v\n", err)
			}
		case "4", "list":
			if err := a.handleList(); err != nil {
				if errors.Is(err, io.EOF) {
					fmt.Println()
					fmt.Println("Exiting interactive mode.")
					return nil
				}
				fmt.Printf("Listing failed: %v\n", err)
			}
		case "5", "explore":
			if err := a.handleExplore(); err != nil {
				if errors.Is(err, io.EOF) {
					fmt.Println()
					fmt.Println("Exiting interactive mode.")
					return nil
				}
				fmt.Printf("Explorer failed: %v\n", err)
			}
		case "6", "exit", "quit", "q":
			fmt.Println()
			fmt.Println("Exiting interactive mode.")
			return nil
		default:
			fmt.Println("Invalid selection. Try again.")
		}
	}
}

func (a *Application) handleTransfer() error {
	fmt.Println()
	fmt.Println("Transfer data between databases")

	sourceCfg, err := a.loadOrPromptConfig("source", "")
	if err != nil {
		return err
	}

	targetCfg, err := a.loadOrPromptConfig("target", sourceCfg.Database.Type)
	if err != nil {
		return err
	}

	schemaOnlyFlag, dataOnlyFlag, workers, batch, verboseFlag, err := a.promptTransferOptions(sourceCfg.Database.Type)
	if err != nil {
		return err
	}

	return a.service.Transfer(sourceCfg, targetCfg, schemaOnlyFlag, dataOnlyFlag, workers, batch, verboseFlag)
}

func (a *Application) handleBackup() error {
	fmt.Println()
	fmt.Println("Create a backup")

	cfg, err := a.loadOrPromptConfig("database", "")
	if err != nil {
		return err
	}

	verboseFlag, err := a.promptYesNo("Enable verbose logging?", false)
	if err != nil {
		return err
	}

	return a.service.Backup(cfg, verboseFlag)
}

func (a *Application) handleRestore() error {
	fmt.Println()
	fmt.Println("Restore a backup")

	cfg, err := a.loadOrPromptConfig("database", "")
	if err != nil {
		return err
	}

	verboseFlag, err := a.promptYesNo("Enable verbose logging?", false)
	if err != nil {
		return err
	}

	return a.service.Restore(cfg, verboseFlag)
}

func (a *Application) handleList() error {
	fmt.Println()
	fmt.Println("List databases on a server")

	cfg, err := a.loadOrPromptConfig("database", "")
	if err != nil {
		return err
	}

	return a.service.ListDatabases(cfg)
}

func (a *Application) handleExplore() error {
	fmt.Println()
	fmt.Println("Explore a database in the console UI")

	cfg, err := a.loadOrPromptConfig("database", "")
	if err != nil {
		return err
	}

	return explorer.Run(cfg)
}

func (a *Application) promptString(label string, required bool) (string, error) {
	for {
		fmt.Printf("%s: ", label)
		input, err := a.readLine()
		if err != nil {
			return "", err
		}
		if input == "" && required {
			fmt.Println("Please provide a value.")
			continue
		}
		return input, nil
	}
}

func (a *Application) promptYesNo(question string, defaultValue bool) (bool, error) {
	suffix := "(y/N)"
	if defaultValue {
		suffix = "(Y/n)"
	}

	for {
		fmt.Printf("%s %s ", question, suffix)
		input, err := a.readLine()
		if err != nil {
			return false, err
		}

		if input == "" {
			return defaultValue, nil
		}

		switch strings.ToLower(input) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Println("Please answer with y or n.")
		}
	}
}

func (a *Application) promptInt(question string, defaultValue int) (int, error) {
	for {
		fmt.Printf("%s [%d]: ", question, defaultValue)
		input, err := a.readLine()
		if err != nil {
			return 0, err
		}

		if input == "" {
			return defaultValue, nil
		}

		value, err := strconv.Atoi(input)
		if err != nil {
			fmt.Println("Please enter a valid number.")
			continue
		}

		return value, nil
	}
}

func (a *Application) loadOrPromptConfig(label, expectedType string) (*config.Config, error) {
	for {
		fmt.Printf("\nConfigure %s connection\n", label)

		if cfg, ok, err := a.selectProfile(expectedType); err != nil {
			return nil, err
		} else if ok {
			return cfg, nil
		}

		dbType := expectedType
		if dbType == "" {
			var err error
			dbType, err = a.promptDatabaseType()
			if err != nil {
				return nil, err
			}
		}

		cfg, err := a.promptManualConfig(dbType, label)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, err
			}
			fmt.Printf("Error: %v\n", err)
			continue
		}

		if err := a.persistConfig(cfg); err != nil {
			fmt.Printf("Warning: failed to save config: %v\n", err)
		}

		return cfg, nil
	}
}

func (a *Application) promptManualConfig(dbType, label string) (*config.Config, error) {
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Type: dbType,
		},
	}

	switch dbType {
	case "postgres":
		fmt.Printf("\nEnter PostgreSQL connection details for %s database:\n", label)

		host, err := a.promptStringWithDefault("Host", "localhost")
		if err != nil {
			return nil, err
		}
		port, err := a.promptInt("Port", 5432)
		if err != nil {
			return nil, err
		}
		dbName, err := a.promptStringWithDefault("Database name", "postgres")
		if err != nil {
			return nil, err
		}
		username, err := a.promptString("Username (leave blank for none)", false)
		if err != nil {
			return nil, err
		}
		password, err := a.promptString("Password (leave blank for none)", false)
		if err != nil {
			return nil, err
		}
		sslMode, err := a.promptStringWithDefault("SSL mode", "disable")
		if err != nil {
			return nil, err
		}

		cfg.Database.Host = host
		cfg.Database.Port = port
		cfg.Database.Database = dbName
		cfg.Database.Username = username
		cfg.Database.Password = password
		cfg.Database.SSLMode = strings.TrimSpace(sslMode)
		if cfg.Database.SSLMode == "" {
			cfg.Database.SSLMode = "disable"
		}

	case "mongo":
		fmt.Printf("\nEnter MongoDB connection details for %s database:\n", label)

		useURI, err := a.promptYesNo("Provide a MongoDB URI?", false)
		if err != nil {
			return nil, err
		}

		if useURI {
			uri, err := a.promptString("MongoDB URI", true)
			if err != nil {
				return nil, err
			}
			cfg.Database.URI = uri
		} else {
			host, err := a.promptStringWithDefault("Host", "localhost")
			if err != nil {
				return nil, err
			}
			port, err := a.promptInt("Port", 27017)
			if err != nil {
				return nil, err
			}
			username, err := a.promptString("Username (leave blank for none)", false)
			if err != nil {
				return nil, err
			}
			password, err := a.promptString("Password (leave blank for none)", false)
			if err != nil {
				return nil, err
			}
			authDB := ""
			if username != "" {
				authDB, err = a.promptStringWithDefault("Auth database", "admin")
				if err != nil {
					return nil, err
				}
			}

			cfg.Database.Host = host
			cfg.Database.Port = port
			cfg.Database.Username = username
			cfg.Database.Password = password
			cfg.Database.AuthDatabase = strings.TrimSpace(authDB)
			if cfg.Database.Port == 0 {
				cfg.Database.Port = 27017
			}
		}

		dbName, err := a.promptStringWithDefault("Database name", "test")
		if err != nil {
			return nil, err
		}
		cfg.Database.Database = dbName

	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}

	return cfg, nil
}

func (a *Application) promptDatabaseType() (string, error) {
	for {
		fmt.Println()
		fmt.Println("Select database type:")
		fmt.Println("1. PostgreSQL")
		fmt.Println("2. MongoDB")
		fmt.Print("Selection: ")

		input, err := a.readLine()
		if err != nil {
			return "", err
		}

		switch strings.ToLower(strings.TrimSpace(input)) {
		case "1", "postgres", "postgresql":
			return "postgres", nil
		case "2", "mongo", "mongodb":
			return "mongo", nil
		default:
			fmt.Println("Please choose 1 or 2.")
		}
	}
}

func (a *Application) promptTransferOptions(dbType string) (bool, bool, int, int, bool, error) {
	var (
		schemaOnly bool
		dataOnly   bool
		err        error
	)

	if dbType == "mongo" {
		schemaOnly, err = a.promptYesNo("Transfer indexes only (skip documents)?", false)
		if err != nil {
			return false, false, 0, 0, false, err
		}
		if !schemaOnly {
			dataOnly, err = a.promptYesNo("Transfer documents only (skip indexes)?", false)
			if err != nil {
				return false, false, 0, 0, false, err
			}
		}
	} else {
		schemaOnly, err = a.promptYesNo("Transfer schema only?", false)
		if err != nil {
			return false, false, 0, 0, false, err
		}
		if !schemaOnly {
			dataOnly, err = a.promptYesNo("Transfer data only?", false)
			if err != nil {
				return false, false, 0, 0, false, err
			}
		}
	}

	workers, err := a.promptInt("Number of parallel workers", 4)
	if err != nil {
		return false, false, 0, 0, false, err
	}

	batch, err := a.promptInt("Batch size", 1000)
	if err != nil {
		return false, false, 0, 0, false, err
	}

	verboseFlag, err := a.promptYesNo("Enable verbose logging?", false)
	if err != nil {
		return false, false, 0, 0, false, err
	}

	return schemaOnly, dataOnly, workers, batch, verboseFlag, nil
}

func (a *Application) promptStringWithDefault(label, defaultValue string) (string, error) {
	for {
		if defaultValue != "" {
			fmt.Printf("%s [%s]: ", label, defaultValue)
		} else {
			fmt.Printf("%s: ", label)
		}

		input, err := a.readLine()
		if err != nil {
			return "", err
		}

		if input == "" {
			if defaultValue != "" {
				return defaultValue, nil
			}
			fmt.Println("Please provide a value.")
			continue
		}

		return input, nil
	}
}

func (a *Application) readLine() (string, error) {
	line, err := a.reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) && len(line) > 0 {
			return strings.TrimSpace(line), nil
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (a *Application) selectProfile(expectedType string) (*config.Config, bool, error) {
	profiles, err := a.profileManager.List(expectedType)
	if err != nil {
		return nil, false, err
	}

	if len(profiles) == 0 {
		return nil, false, nil
	}

	for {
		fmt.Println("Saved configurations:")
		for i, profile := range profiles {
			label := profile.Name
			if profile.Type != "" {
				label = fmt.Sprintf("%s (%s)", label, profile.Type)
			}
			fmt.Printf("  %d) %s\n", i+1, label)
		}
		fmt.Println("  n) Create a new configuration")

		choice, err := a.promptString("Select a configuration (number) or 'n'", true)
		if err != nil {
			return nil, false, err
		}

		choice = strings.ToLower(strings.TrimSpace(choice))
		if choice == "n" || choice == "new" {
			return nil, false, nil
		}

		index, err := strconv.Atoi(choice)
		if err != nil || index < 1 || index > len(profiles) {
			fmt.Println("Please choose a valid option.")
			continue
		}

		cfg, err := config.LoadConfig(profiles[index-1].Path)
		if err != nil {
			fmt.Printf("Failed to load %s: %v\n", profiles[index-1].Name, err)
			continue
		}

		return cfg, true, nil
	}
}

func (a *Application) persistConfig(cfg *config.Config) error {
	save, err := a.promptYesNo("Save this configuration for future use?", true)
	if err != nil || !save {
		return err
	}

	defaultName := fmt.Sprintf("%s-%s_%s", cfg.Database.Type, cfg.Database.Host, time.Now().Format("20060102_150405"))
	name, err := a.promptStringWithDefault("Configuration name", defaultName)
	if err != nil {
		return err
	}

	_, err = a.profileManager.Save(name, cfg)
	return err
}
