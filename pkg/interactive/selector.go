package interactive

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kadirbelkuyu/DBRTS/internal/backup"
)

type DatabaseSelector struct {
	reader *bufio.Reader
	dbType string
}

func NewDatabaseSelector(dbType string) *DatabaseSelector {
	return &DatabaseSelector{
		reader: bufio.NewReader(os.Stdin),
		dbType: strings.ToLower(strings.TrimSpace(dbType)),
	}
}

func (ds *DatabaseSelector) SelectDatabase(databases []backup.DatabaseInfo) (*backup.DatabaseInfo, error) {
	if len(databases) == 0 {
		return nil, fmt.Errorf("no databases found")
	}

	fmt.Println()
	fmt.Println("Available databases:")
	fmt.Println(strings.Repeat("=", 80))

	switch ds.dbType {
	case "mongo":
		fmt.Printf("%-4s %-30s %-15s %-15s\n", "No", "Database", "Collections", "Size")
		fmt.Println(strings.Repeat("-", 80))
		for i, db := range databases {
			fmt.Printf("%-4d %-30s %-15d %-15s\n", i+1, db.Name, db.Collections, safeValue(db.Size, "n/a"))
		}
	default:
		fmt.Printf("%-4s %-30s %-15s %-15s %-15s\n", "No", "Database", "Owner", "Encoding", "Size")
		fmt.Println(strings.Repeat("-", 80))
		for i, db := range databases {
			fmt.Printf("%-4d %-30s %-15s %-15s %-15s\n",
				i+1, db.Name, safeValue(db.Owner, "n/a"), safeValue(db.Encoding, "n/a"), safeValue(db.Size, "n/a"))
		}
	}

	fmt.Println(strings.Repeat("=", 80))

	for {
		fmt.Printf("\nSelect the database number (1-%d): ", len(databases))

		input, err := ds.reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("unable to read input: %w", err)
		}

		input = strings.TrimSpace(input)

		if input == "" {
			fmt.Println("Please enter a number.")
			continue
		}

		choice, err := strconv.Atoi(input)
		if err != nil {
			fmt.Println("Please enter a valid number.")
			continue
		}

		if choice < 1 || choice > len(databases) {
			fmt.Printf("Please select a number between 1 and %d.\n", len(databases))
			continue
		}

		selected := &databases[choice-1]
		fmt.Printf("\nSelected database: %s\n", selected.Name)
		return selected, nil
	}
}

func (ds *DatabaseSelector) ConfirmAction(action, target string) bool {
	fmt.Printf("\nConfirm running %s for %s (y/N): ", action, target)

	input, err := ds.reader.ReadString('\n')
	if err != nil {
		return false
	}

	input = strings.ToLower(strings.TrimSpace(input))
	return input == "y" || input == "yes"
}

func (ds *DatabaseSelector) GetBackupOptions(dbType string) backup.BackupOptions {
	dbType = strings.ToLower(strings.TrimSpace(dbType))
	if dbType == "" {
		dbType = ds.dbType
	}

	options := backup.BackupOptions{
		Format:      "custom",
		Compression: 6,
		Verbose:     true,
	}

	if dbType == "mongo" {
		fmt.Println()
		fmt.Println("Backup options (MongoDB):")
		fmt.Println("1. Archive format (.archive)")
		fmt.Println("2. Compressed archive (.archive.gz)")

		for {
			fmt.Print("\nChoose archive type (1-2) [2]: ")
			input, _ := ds.reader.ReadString('\n')
			input = strings.TrimSpace(input)

			if input == "" {
				input = "2"
			}

			switch input {
			case "1":
				options.Format = "archive"
				options.Compression = 0
			case "2":
				options.Format = "archive"
				options.Compression = 1
			default:
				fmt.Println("Please choose 1 or 2.")
				continue
			}
			break
		}
	} else {
		fmt.Println()
		fmt.Println("Backup options (PostgreSQL):")
		fmt.Println("1. SQL format (plain text)")
		fmt.Println("2. Custom format (compressed, recommended)")
		fmt.Println("3. Tar format")
		fmt.Println("4. Directory format")

		for {
			fmt.Print("\nSelect format (1-4) [2]: ")
			input, _ := ds.reader.ReadString('\n')
			input = strings.TrimSpace(input)

			if input == "" {
				input = "2"
			}

			switch input {
			case "1":
				options.Format = "sql"
			case "2":
				options.Format = "custom"
			case "3":
				options.Format = "tar"
			case "4":
				options.Format = "directory"
			default:
				fmt.Println("Please choose a value between 1 and 4.")
				continue
			}
			break
		}

		if options.Format == "custom" || options.Format == "tar" {
			fmt.Print("Compression level (0-9) [6]: ")
			compressionInput, _ := ds.reader.ReadString('\n')
			compressionInput = strings.TrimSpace(compressionInput)

			if compressionInput != "" {
				if comp, err := strconv.Atoi(compressionInput); err == nil && comp >= 0 && comp <= 9 {
					options.Compression = comp
				}
			}
		}

		fmt.Print("Backup schema only? (y/N): ")
		schemaInput, _ := ds.reader.ReadString('\n')
		schemaInput = strings.ToLower(strings.TrimSpace(schemaInput))
		options.SchemaOnly = schemaInput == "y" || schemaInput == "yes"

		if !options.SchemaOnly {
			fmt.Print("Backup data only? (y/N): ")
			dataInput, _ := ds.reader.ReadString('\n')
			dataInput = strings.ToLower(strings.TrimSpace(dataInput))
			options.DataOnly = dataInput == "y" || dataInput == "yes"
		}
	}

	fmt.Print("Output path (leave empty to auto-create under backup/): ")
	outputInput, _ := ds.reader.ReadString('\n')
	options.OutputPath = strings.TrimSpace(outputInput)

	return options
}

func (ds *DatabaseSelector) GetRestoreOptions(dbType string) backup.RestoreOptions {
	dbType = strings.ToLower(strings.TrimSpace(dbType))
	if dbType == "" {
		dbType = ds.dbType
	}

	options := backup.RestoreOptions{
		Verbose:     true,
		ExitOnError: true,
	}

	fmt.Print("Backup file path (look under backup/): ")
	backupInput, _ := ds.reader.ReadString('\n')
	options.BackupPath = strings.TrimSpace(backupInput)

	fmt.Print("Target database name: ")
	dbInput, _ := ds.reader.ReadString('\n')
	options.TargetDatabase = strings.TrimSpace(dbInput)

	if dbType == "postgres" {
		fmt.Print("Create the database if it does not exist? (Y/n): ")
		createInput, _ := ds.reader.ReadString('\n')
		createInput = strings.ToLower(strings.TrimSpace(createInput))
		options.CreateDatabase = createInput != "n" && createInput != "no"

		fmt.Print("Drop existing objects before restore? (y/N): ")
		cleanInput, _ := ds.reader.ReadString('\n')
		cleanInput = strings.ToLower(strings.TrimSpace(cleanInput))
		options.CleanFirst = cleanInput == "y" || cleanInput == "yes"

		fmt.Print("Stop on first error? (Y/n): ")
		errorInput, _ := ds.reader.ReadString('\n')
		errorInput = strings.ToLower(strings.TrimSpace(errorInput))
		options.ExitOnError = errorInput != "n" && errorInput != "no"
	} else {
		fmt.Print("Drop collections before restore? (y/N): ")
		cleanInput, _ := ds.reader.ReadString('\n')
		cleanInput = strings.ToLower(strings.TrimSpace(cleanInput))
		options.CleanFirst = cleanInput == "y" || cleanInput == "yes"

		fmt.Print("Stop on first error? (Y/n): ")
		errorInput, _ := ds.reader.ReadString('\n')
		errorInput = strings.ToLower(strings.TrimSpace(errorInput))
		options.ExitOnError = errorInput != "n" && errorInput != "no"

		// MongoDB creates databases on demand.
		options.CreateDatabase = true
	}

	return options
}

func safeValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
