package graph

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func countBooks(
	ctx context.Context,
	collection *mongo.Collection,
	filter bson.M,
) (int32, error) {
	count, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return 0, err
	}

	return int32(count), nil
}
