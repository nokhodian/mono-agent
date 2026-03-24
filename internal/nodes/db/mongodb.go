package dbnodes

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/monoes/monoes-agent/internal/workflow"
)

// MongoDBNode executes MongoDB operations.
// Type: "db.mongodb"
type MongoDBNode struct{}

func (n *MongoDBNode) Type() string { return "db.mongodb" }

func (n *MongoDBNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	operation, _ := config["operation"].(string)
	if operation == "" {
		operation = "find"
	}

	connStr, _ := config["connection_string"].(string)
	if connStr == "" {
		return nil, fmt.Errorf("db.mongodb: 'connection_string' is required")
	}

	database, _ := config["database"].(string)
	if database == "" {
		return nil, fmt.Errorf("db.mongodb: 'database' is required")
	}

	collection, _ := config["collection"].(string)
	if collection == "" {
		return nil, fmt.Errorf("db.mongodb: 'collection' is required")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(connStr))
	if err != nil {
		return nil, fmt.Errorf("db.mongodb: connect failed: %w", err)
	}
	defer client.Disconnect(ctx) //nolint:errcheck

	coll := client.Database(database).Collection(collection)

	// Convert map[string]interface{} to bson.D
	filterMap, _ := config["filter"].(map[string]interface{})
	filter := mapToBSON(filterMap)

	updateMap, _ := config["update"].(map[string]interface{})
	update := mapToBSON(updateMap)

	limitVal := 0
	if v, ok := config["limit"].(float64); ok {
		limitVal = int(v)
	}

	switch operation {
	case "find":
		findOpts := options.Find()
		if limitVal > 0 {
			findOpts.SetLimit(int64(limitVal))
		}
		cursor, err := coll.Find(ctx, filter, findOpts)
		if err != nil {
			return nil, fmt.Errorf("db.mongodb: find failed: %w", err)
		}
		defer cursor.Close(ctx)

		var items []workflow.Item
		for cursor.Next(ctx) {
			var doc map[string]interface{}
			if err := cursor.Decode(&doc); err != nil {
				return nil, fmt.Errorf("db.mongodb: decode failed: %w", err)
			}
			normalizeMongoDoc(doc)
			items = append(items, workflow.NewItem(doc))
		}
		if err := cursor.Err(); err != nil {
			return nil, fmt.Errorf("db.mongodb: cursor error: %w", err)
		}
		return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil

	case "insert_one":
		docMap, _ := config["document"].(map[string]interface{})
		if docMap == nil {
			return nil, fmt.Errorf("db.mongodb: 'document' is required for insert_one")
		}
		result, err := coll.InsertOne(ctx, docMap)
		if err != nil {
			return nil, fmt.Errorf("db.mongodb: insert_one failed: %w", err)
		}
		item := workflow.NewItem(map[string]interface{}{
			"inserted_id": fmt.Sprintf("%v", result.InsertedID),
		})
		return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{item}}}, nil

	case "insert_many":
		var docs []interface{}
		if rawDocs, ok := config["document"].([]interface{}); ok {
			docs = rawDocs
		} else if docMap, ok := config["document"].(map[string]interface{}); ok {
			docs = []interface{}{docMap}
		}
		if len(docs) == 0 {
			return nil, fmt.Errorf("db.mongodb: 'document' is required for insert_many")
		}
		result, err := coll.InsertMany(ctx, docs)
		if err != nil {
			return nil, fmt.Errorf("db.mongodb: insert_many failed: %w", err)
		}
		ids := make([]interface{}, len(result.InsertedIDs))
		for i, id := range result.InsertedIDs {
			ids[i] = fmt.Sprintf("%v", id)
		}
		item := workflow.NewItem(map[string]interface{}{
			"inserted_ids":   ids,
			"inserted_count": len(ids),
		})
		return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{item}}}, nil

	case "update_one":
		result, err := coll.UpdateOne(ctx, filter, update)
		if err != nil {
			return nil, fmt.Errorf("db.mongodb: update_one failed: %w", err)
		}
		item := workflow.NewItem(map[string]interface{}{
			"matched_count":  result.MatchedCount,
			"modified_count": result.ModifiedCount,
			"upserted_count": result.UpsertedCount,
		})
		return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{item}}}, nil

	case "update_many":
		result, err := coll.UpdateMany(ctx, filter, update)
		if err != nil {
			return nil, fmt.Errorf("db.mongodb: update_many failed: %w", err)
		}
		item := workflow.NewItem(map[string]interface{}{
			"matched_count":  result.MatchedCount,
			"modified_count": result.ModifiedCount,
			"upserted_count": result.UpsertedCount,
		})
		return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{item}}}, nil

	case "delete_one":
		result, err := coll.DeleteOne(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("db.mongodb: delete_one failed: %w", err)
		}
		item := workflow.NewItem(map[string]interface{}{
			"deleted_count": result.DeletedCount,
		})
		return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{item}}}, nil

	case "delete_many":
		result, err := coll.DeleteMany(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("db.mongodb: delete_many failed: %w", err)
		}
		item := workflow.NewItem(map[string]interface{}{
			"deleted_count": result.DeletedCount,
		})
		return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{item}}}, nil

	case "aggregate":
		rawPipeline, _ := config["pipeline"].([]interface{})
		pipeline := make(bson.A, len(rawPipeline))
		for i, stage := range rawPipeline {
			if m, ok := stage.(map[string]interface{}); ok {
				pipeline[i] = mapToBSON(m)
			} else {
				pipeline[i] = stage
			}
		}
		cursor, err := coll.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, fmt.Errorf("db.mongodb: aggregate failed: %w", err)
		}
		defer cursor.Close(ctx)

		var items []workflow.Item
		for cursor.Next(ctx) {
			var doc map[string]interface{}
			if err := cursor.Decode(&doc); err != nil {
				return nil, fmt.Errorf("db.mongodb: decode failed: %w", err)
			}
			normalizeMongoDoc(doc)
			items = append(items, workflow.NewItem(doc))
		}
		if err := cursor.Err(); err != nil {
			return nil, fmt.Errorf("db.mongodb: cursor error: %w", err)
		}
		return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil

	default:
		return nil, fmt.Errorf("db.mongodb: unknown operation %q", operation)
	}
}

// mapToBSON converts a map[string]interface{} to bson.D.
func mapToBSON(m map[string]interface{}) bson.D {
	if m == nil {
		return bson.D{}
	}
	d := make(bson.D, 0, len(m))
	for k, v := range m {
		d = append(d, bson.E{Key: k, Value: v})
	}
	return d
}

// normalizeMongoDoc converts bson-specific types (like ObjectID) to strings for JSON serialization.
func normalizeMongoDoc(doc map[string]interface{}) {
	for k, v := range doc {
		switch val := v.(type) {
		case bson.ObjectID:
			doc[k] = val.Hex()
		case map[string]interface{}:
			normalizeMongoDoc(val)
		}
	}
}
