package transfer

import (
	"context"
	"fmt"
	"time"

	"github.com/kadirbelkuyu/DBRTS/internal/config"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type mongoEngine struct {
	sourceConfig *config.Config
	targetConfig *config.Config
	options      Options
	sourceClient *mongo.Client
	targetClient *mongo.Client
}

func newMongoEngine(sourceConfig, targetConfig *config.Config, options Options) (*mongoEngine, error) {
	engine := &mongoEngine{
		sourceConfig: sourceConfig,
		targetConfig: targetConfig,
		options:      options,
	}
	return engine, nil
}

func (e *mongoEngine) Execute() error {
	e.options.Logger.Info("Starting MongoDB transfer...")

	if err := e.connect(); err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}
	defer e.cleanup()

	if err := e.transfer(); err != nil {
		return err
	}

	e.options.Logger.Info("MongoDB transfer completed successfully.")
	return nil
}

func (e *mongoEngine) connect() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sourceClient, err := mongo.Connect(ctx, options.Client().ApplyURI(e.sourceConfig.GetMongoURI()))
	if err != nil {
		return fmt.Errorf("failed to connect to source MongoDB: %w", err)
	}
	if err := sourceClient.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("failed to ping source MongoDB: %w", err)
	}

	targetClient, err := mongo.Connect(ctx, options.Client().ApplyURI(e.targetConfig.GetMongoURI()))
	if err != nil {
		return fmt.Errorf("failed to connect to target MongoDB: %w", err)
	}
	if err := targetClient.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("failed to ping target MongoDB: %w", err)
	}

	e.sourceClient = sourceClient
	e.targetClient = targetClient
	return nil
}

func (e *mongoEngine) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if e.sourceClient != nil {
		_ = e.sourceClient.Disconnect(ctx)
	}
	if e.targetClient != nil {
		_ = e.targetClient.Disconnect(ctx)
	}
}

func (e *mongoEngine) transfer() error {
	sourceDBName := e.sourceConfig.Database.Database
	targetDBName := e.targetConfig.Database.Database

	if sourceDBName == "" || targetDBName == "" {
		return fmt.Errorf("source and target database names are required for MongoDB transfer")
	}

	ctx := context.Background()

	sourceDB := e.sourceClient.Database(sourceDBName)
	targetDB := e.targetClient.Database(targetDBName)

	collections, err := sourceDB.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return fmt.Errorf("failed to list collections: %w", err)
	}

	copyIndexes := !e.options.DataOnly
	copyData := !e.options.SchemaOnly

	if !copyIndexes && !copyData {
		e.options.Logger.Warn("Both schema-only and data-only flags are set to skip operations; nothing to do.")
		return nil
	}

	for _, collectionName := range collections {
		if err := e.cloneCollection(ctx, sourceDB, targetDB, collectionName, copyIndexes, copyData); err != nil {
			return err
		}
	}

	return nil
}

func (e *mongoEngine) cloneCollection(
	ctx context.Context,
	sourceDB *mongo.Database,
	targetDB *mongo.Database,
	collectionName string,
	copyIndexes bool,
	copyData bool,
) error {
	e.options.Logger.Infof("Transferring collection %s...", collectionName)

	sourceCollection := sourceDB.Collection(collectionName)
	targetCollection := targetDB.Collection(collectionName)

	if err := targetCollection.Drop(ctx); err != nil {
		if !isNamespaceNotFound(err) {
			return fmt.Errorf("failed to drop target collection %s: %w", collectionName, err)
		}
	}

	if copyIndexes {
		if err := e.cloneIndexes(ctx, sourceCollection, targetCollection); err != nil {
			return fmt.Errorf("failed to clone indexes for %s: %w", collectionName, err)
		}
	}

	if !copyData {
		return nil
	}

	batchSize := e.options.BatchSize
	if batchSize <= 0 {
		batchSize = 500
	}

	cursor, err := sourceCollection.Find(ctx, bson.D{})
	if err != nil {
		return fmt.Errorf("failed to query collection %s: %w", collectionName, err)
	}
	defer cursor.Close(ctx)

	batch := make([]interface{}, 0, batchSize)
	for cursor.Next(ctx) {
		var document bson.M
		if err := cursor.Decode(&document); err != nil {
			return fmt.Errorf("failed to decode document from %s: %w", collectionName, err)
		}

		batch = append(batch, document)
		if len(batch) >= batchSize {
			if err := e.insertBatch(ctx, targetCollection, batch); err != nil {
				return fmt.Errorf("failed to insert batch into %s: %w", collectionName, err)
			}
			batch = batch[:0]
		}
	}

	if err := cursor.Err(); err != nil {
		return fmt.Errorf("error reading documents from %s: %w", collectionName, err)
	}

	if len(batch) > 0 {
		if err := e.insertBatch(ctx, targetCollection, batch); err != nil {
			return fmt.Errorf("failed to insert final batch into %s: %w", collectionName, err)
		}
	}

	return nil
}

func (e *mongoEngine) cloneIndexes(ctx context.Context, sourceCollection, targetCollection *mongo.Collection) error {
	cursor, err := sourceCollection.Indexes().List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list indexes: %w", err)
	}
	defer cursor.Close(ctx)

	var models []mongo.IndexModel
	for cursor.Next(ctx) {
		var indexDoc struct {
			Name   string      `bson:"name"`
			Key    bson.D      `bson:"key"`
			Unique bool        `bson:"unique,omitempty"`
			Sparse bool        `bson:"sparse,omitempty"`
			Expire int32       `bson:"expireAfterSeconds,omitempty"`
			Bits   interface{} `bson:"bits,omitempty"`
			Type   interface{} `bson:"2dsphereIndexVersion,omitempty"`
			Other  bson.M      `bson:",inline"`
		}
		if err := cursor.Decode(&indexDoc); err != nil {
			return fmt.Errorf("failed to decode index: %w", err)
		}

		if indexDoc.Name == "_id_" {
			continue
		}

		indexOptions := options.Index().SetName(indexDoc.Name)
		if indexDoc.Unique {
			indexOptions = indexOptions.SetUnique(true)
		}
		if indexDoc.Sparse {
			indexOptions = indexOptions.SetSparse(true)
		}
		if indexDoc.Expire != 0 {
			indexOptions = indexOptions.SetExpireAfterSeconds(indexDoc.Expire)
		}

		models = append(models, mongo.IndexModel{
			Keys:    indexDoc.Key,
			Options: indexOptions,
		})
	}

	if err := cursor.Err(); err != nil {
		return fmt.Errorf("error reading indexes: %w", err)
	}

	if len(models) == 0 {
		return nil
	}

	if _, err := targetCollection.Indexes().CreateMany(ctx, models); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	return nil
}

func (e *mongoEngine) insertBatch(ctx context.Context, collection *mongo.Collection, batch []interface{}) error {
	if len(batch) == 0 {
		return nil
	}

	opts := options.InsertMany().SetOrdered(false)
	_, err := collection.InsertMany(ctx, batch, opts)
	return err
}

func isNamespaceNotFound(err error) bool {
	cmdErr, ok := err.(mongo.CommandError)
	return ok && cmdErr.Code == 26
}
