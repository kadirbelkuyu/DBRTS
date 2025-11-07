package transfer

import (
	"fmt"

	"github.com/kadirbelkuyu/DBRTS/internal/config"
	"github.com/kadirbelkuyu/DBRTS/pkg/logger"
)

type Options struct {
	SchemaOnly      bool
	DataOnly        bool
	ParallelWorkers int
	BatchSize       int
	Logger          *logger.Logger
}

type Engine interface {
	Execute() error
}

type Service struct {
	engine Engine
}

func NewService(sourceConfig, targetConfig *config.Config, options Options) (*Service, error) {
	sourceType := sourceConfig.Database.Type
	targetType := targetConfig.Database.Type

	if sourceType != targetType {
		return nil, fmt.Errorf("cross-engine transfers are not supported between %s and %s", sourceType, targetType)
	}

	var engine Engine
	switch sourceType {
	case "postgres":
		engine = newPostgresEngine(sourceConfig, targetConfig, options)
	case "mongo":
		mongoEngine, err := newMongoEngine(sourceConfig, targetConfig, options)
		if err != nil {
			return nil, err
		}
		engine = mongoEngine
	default:
		return nil, fmt.Errorf("unsupported database type: %s", sourceType)
	}

	return &Service{engine: engine}, nil
}

func (s *Service) Execute() error {
	return s.engine.Execute()
}
