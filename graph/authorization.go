package graph

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func userTargetID(ctx context.Context, requestedID string) (bson.ObjectID, error) {
	if isAdmin(ctx) {
		oid, err := bson.ObjectIDFromHex(requestedID)
		if err != nil {
			return bson.ObjectID{}, fmt.Errorf("invalid user ID")
		}
		return oid, nil
	}

	return authenticatedUserID(ctx)
}
