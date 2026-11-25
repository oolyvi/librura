package graph

import (
	"example/booklibrary/graph/model"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type BookDocument struct {
	ID          bson.ObjectID `bson:"_id,omitempty"`
	UserID      bson.ObjectID `bson:"userId"`
	Title       string        `bson:"title"`
	Author      string        `bson:"author"`
	Status      string        `bson:"status"`
	IsFavorite  bool          `bson:"isFavorite"`
	PageCount   *int32        `bson:"pageCount,omitempty"`
	CurrentPage *int32        `bson:"currentPage,omitempty"`
	CompletedAt *time.Time    `bson:"completedAt,omitempty"`
	Quotes      []string      `bson:"quotes,omitempty"`
	Price       *float64      `bson:"price,omitempty"`
	Review      *string       `bson:"review,omitempty"`
	Rating      int32         `bson:"rating"`
	CreatedAt   time.Time     `bson:"createdAt"`
	UpdatedAt   time.Time     `bson:"updatedAt"`
}

func toBookModel(doc BookDocument) *model.Book {
	return &model.Book{
		ID:          doc.ID.Hex(),
		UserID:      doc.UserID.Hex(),
		Title:       doc.Title,
		Author:      doc.Author,
		Status:      model.BookStatus(doc.Status),
		IsFavorite:  doc.IsFavorite,
		PageCount:   doc.PageCount,
		CurrentPage: doc.CurrentPage,
		Quotes:      doc.Quotes,
		Price:       doc.Price,
		Review:      doc.Review,
		Rating:      doc.Rating,
		CreatedAt:   doc.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   doc.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
