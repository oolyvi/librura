package graph

import (
	"context"

	"example/booklibrary/graph/model"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (r *queryResolver) findBooks(ctx context.Context, filter bson.M) ([]*model.Book, error) {
	ownedFilter := bson.M{}
	if !isAdmin(ctx) {
		userID, err := authenticatedUserID(ctx)
		if err != nil {
			return nil, err
		}
		ownedFilter["userId"] = userID
	}
	for key, value := range filter {
		ownedFilter[key] = value
	}
	return r.cachedBooks(ctx, bookListCacheKey(ownedFilter), func() ([]*model.Book, error) {
		return r.loadBooks(ctx, ownedFilter)
	})
}

func (r *queryResolver) loadBooks(ctx context.Context, filter bson.M) ([]*model.Book, error) {
	cursor, err := r.DB.Collection("books").Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var docs []BookDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	books := make([]*model.Book, 0, len(docs))
	for _, doc := range docs {
		books = append(books, toBookModel(doc))
	}
	return books, nil
}

func findOwnedBook(ctx context.Context, db *mongo.Database, oid, userID bson.ObjectID) (*model.Book, error) {
	var doc BookDocument
	err := db.Collection("books").FindOne(ctx, bson.M{"_id": oid, "userId": userID}).Decode(&doc)
	if err != nil {
		return nil, err
	}
	return toBookModel(doc), nil
}
