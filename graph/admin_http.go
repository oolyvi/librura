package graph

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func AdminHandler(resolver *Resolver) http.Handler {
	db := resolver.DB
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/books", func(w http.ResponseWriter, r *http.Request) {
		if err := requireAdmin(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		var in struct {
			UserID      string   `json:"userId"`
			Title       string   `json:"title"`
			Author      string   `json:"author"`
			Status      string   `json:"status"`
			IsFavorite  bool     `json:"isFavorite"`
			PageCount   *int32   `json:"pageCount"`
			CurrentPage *int32   `json:"currentPage"`
			Quotes      []string `json:"quotes"`
			Price       *float64 `json:"price"`
			Review      *string  `json:"review"`
			Rating      int32    `json:"rating"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		ownerID, err := bson.ObjectIDFromHex(in.UserID)
		if err != nil {
			http.Error(w, "invalid owner user ID", http.StatusBadRequest)
			return
		}
		var owner UserDocument
		if err := db.Collection("users").FindOne(r.Context(), bson.M{"_id": ownerID}).Decode(&owner); err != nil {
			http.Error(w, "owner user not found", http.StatusNotFound)
			return
		}
		in.Title, in.Author = strings.TrimSpace(in.Title), strings.TrimSpace(in.Author)
		if in.Title == "" || in.Author == "" {
			http.Error(w, "title and author are required", http.StatusBadRequest)
			return
		}
		if err := validateBookText(&in.Title, &in.Author, in.Review, in.Quotes); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if in.Status == "" {
			in.Status = "WANT_TO_READ"
		}
		if in.Status != "WANT_TO_READ" && in.Status != "READING" && in.Status != "COMPLETED" {
			http.Error(w, "invalid book status", http.StatusBadRequest)
			return
		}
		if in.Rating < 0 || in.Rating > 5 {
			http.Error(w, "rating must be between 0 and 5", http.StatusBadRequest)
			return
		}
		if in.PageCount != nil && *in.PageCount < 0 {
			http.Error(w, "page count must not be negative", http.StatusBadRequest)
			return
		}
		if in.CurrentPage != nil && (*in.CurrentPage < 0 || (in.PageCount != nil && *in.CurrentPage > *in.PageCount)) {
			http.Error(w, "current page is invalid", http.StatusBadRequest)
			return
		}
		now := time.Now().UTC()
		doc := BookDocument{UserID: ownerID, Title: in.Title, Author: in.Author, Status: in.Status, IsFavorite: in.IsFavorite, PageCount: in.PageCount, CurrentPage: in.CurrentPage, Quotes: in.Quotes, Price: in.Price, Review: in.Review, Rating: in.Rating, CreatedAt: now, UpdatedAt: now}
		if in.Status == "COMPLETED" {
			doc.CompletedAt = &now
		}
		result, err := db.Collection("books").InsertOne(r.Context(), doc)
		if err != nil {
			http.Error(w, "could not create book", http.StatusInternalServerError)
			return
		}
		doc.ID = result.InsertedID.(bson.ObjectID)
		resolver.invalidateBookCache(r.Context())
		resolver.publishBookEvent("book.created", doc.ID.Hex())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(toBookModel(doc))
	})
	mux.HandleFunc("GET /admin/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := requireAdmin(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		oid, err := bson.ObjectIDFromHex(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid user ID", http.StatusBadRequest)
			return
		}
		var user UserDocument
		if err := db.Collection("users").FindOne(r.Context(), bson.M{"_id": oid}).Decode(&user); err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": user.ID.Hex(), "email": user.Email, "role": user.Role, "yearlyReadingGoal": user.YearlyReadingGoal, "createdAt": user.CreatedAt, "updatedAt": user.UpdatedAt})
	})
	mux.HandleFunc("POST /admin/users/role", func(w http.ResponseWriter, r *http.Request) {
		if err := requireAdmin(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		var in struct {
			UserID string `json:"userId"`
			Role   string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if in.Role != RoleUser && in.Role != RoleAdmin {
			http.Error(w, "role must be USER or ADMIN", http.StatusBadRequest)
			return
		}
		adminID, _ := authenticatedUserID(r.Context())
		targetID, err := bson.ObjectIDFromHex(in.UserID)
		if err != nil {
			http.Error(w, "invalid user ID", http.StatusBadRequest)
			return
		}
		if targetID == adminID {
			http.Error(w, "you cannot change your own administrator role", http.StatusBadRequest)
			return
		}
		res, err := db.Collection("users").UpdateOne(r.Context(), bson.M{"_id": targetID}, bson.M{"$set": bson.M{"role": in.Role, "updatedAt": time.Now().UTC()}})
		if err != nil {
			http.Error(w, "could not update role", http.StatusInternalServerError)
			return
		}
		if res.MatchedCount == 0 {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /admin/users/{id}/goal", func(w http.ResponseWriter, r *http.Request) {
		if err := requireAdmin(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		oid, err := bson.ObjectIDFromHex(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid user ID", http.StatusBadRequest)
			return
		}
		var user UserDocument
		if err := db.Collection("users").FindOne(r.Context(), bson.M{"_id": oid}).Decode(&user); err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int32{"goal": user.YearlyReadingGoal})
	})
	mux.HandleFunc("POST /admin/users/goal", func(w http.ResponseWriter, r *http.Request) {
		if err := requireAdmin(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		var in struct {
			UserID string `json:"userId"`
			Goal   int32  `json:"goal"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		oid, err := bson.ObjectIDFromHex(in.UserID)
		if err != nil || in.Goal < 0 {
			http.Error(w, "invalid user ID or goal", http.StatusBadRequest)
			return
		}
		res, err := db.Collection("users").UpdateOne(r.Context(), bson.M{"_id": oid}, bson.M{"$set": bson.M{"yearlyReadingGoal": in.Goal, "updatedAt": time.Now().UTC()}})
		if err != nil {
			http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
			return
		}
		if res.MatchedCount == 0 {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return AuthMiddleware(db, mux)
}
