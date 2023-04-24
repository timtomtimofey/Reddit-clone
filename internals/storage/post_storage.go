package storage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	StatusCommentOK = iota
	StatusError
	StatusCommentNotExist
	StatusPostNotExist
)

// All information about post in a single document, so no normalization
// If for example user will change a username, then all his posts and comments should be updated
type PostStorageImpl struct {
	posts *mongo.Collection
	users *sql.DB
}

func NewPostStorage(posts *mongo.Collection, users *sql.DB) PostStorage {
	return &PostStorageImpl{
		posts: posts,
		users: users,
	}
}

func (ps *PostStorageImpl) GetPost(postID string, ctx context.Context) (*Post, error) {
	hexPostID, err := primitive.ObjectIDFromHex(postID)
	if err != nil {
		return nil, err
	}
	post := &Post{}
	err = ps.posts.FindOneAndUpdate(ctx, bson.M{"_id": hexPostID}, bson.M{"$inc": bson.M{"views": 1}}).Decode(post)
	return post, err
}

func (ps *PostStorageImpl) GetPosts(ctx context.Context) ([]*Post, error) {
	cursor, err := ps.posts.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	return getPostsByCursor(cursor, ctx)
}

func (ps *PostStorageImpl) GetPostsByCategory(category string, ctx context.Context) ([]*Post, error) {
	cursor, err := ps.posts.Find(ctx, bson.M{"category": category})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	return getPostsByCursor(cursor, ctx)
}

func (ps *PostStorageImpl) GetPostsByUsername(username string, ctx context.Context) ([]*Post, error) {
	cursor, err := ps.posts.Find(ctx, bson.M{"author.username": username})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	return getPostsByCursor(cursor, ctx)
}

func getPostsByCursor(cursor *mongo.Cursor, ctx context.Context) ([]*Post, error) {
	posts := make([]*Post, 0, 10)
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		post := &Post{}
		if err := cursor.Decode(post); err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	return posts, nil
}

func (ps *PostStorageImpl) CheckPostExist(postID string, ctx context.Context) (bool, error) {
	hexPostID, err := primitive.ObjectIDFromHex(postID)
	if err != nil {
		return false, err
	}
	if err := ps.posts.FindOne(ctx, bson.M{"_id": hexPostID}).Err(); err == mongo.ErrNoDocuments {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func (ps *PostStorageImpl) CheckCommentExist(postID, commentID string, ctx context.Context) (int, error) {
	hexPostID, err := primitive.ObjectIDFromHex(postID)
	if err != nil {
		return StatusError, err
	}
	post := &Post{}
	hexCommentID, err := primitive.ObjectIDFromHex(commentID)
	if err != nil {
		return StatusError, err
	}
	if err := ps.posts.FindOne(ctx, bson.M{"_id": hexPostID}).Decode(post); err == mongo.ErrNoDocuments {
		return StatusPostNotExist, nil
	} else if err != nil {
		return StatusError, err
	}
	for _, comment := range post.Comments {
		if bytes.Equal(comment.ID[:], hexCommentID[:]) {
			return StatusCommentOK, nil
		}
	}
	return StatusCommentNotExist, nil
}

func (ps *PostStorageImpl) CheckUserExist(username string, ctx context.Context) (bool, error) {
	var exist bool
	err := ps.users.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = $1);", username).Scan(&exist)
	return exist, err
}

func (ps *PostStorageImpl) CheckPostOwner(postID string, user User, ctx context.Context) (bool, error) {
	log.Printf("%v\n", user)
	hexPostID, err := primitive.ObjectIDFromHex(postID)
	if err != nil {
		return false, err
	}
	err = ps.posts.FindOne(ctx, bson.M{"_id": hexPostID, "author._id": user.UserID}).Err()
	if err == mongo.ErrNoDocuments {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func (ps *PostStorageImpl) CheckCommentOwner(postID, commentID string, user User, ctx context.Context) (bool, error) {
	hexPostID, err := primitive.ObjectIDFromHex(postID)
	if err != nil {
		return false, err
	}
	hexCommentID, err := primitive.ObjectIDFromHex(commentID)
	if err != nil {
		return false, err
	}
	filter := bson.M{
		"_id": hexPostID,
		"comments": bson.M{"$elemMatch": bson.M{
			"author._id": user.UserID,
			"_id":        hexCommentID,
		}},
	}
	if err := ps.posts.FindOne(ctx, filter).Err(); err == mongo.ErrNoDocuments {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func (ps *PostStorageImpl) MakeComment(postID, comment string, user User, ctx context.Context) error {
	hexPostID, err := primitive.ObjectIDFromHex(postID)
	if err != nil {
		return err
	}
	update := bson.M{
		"$push": bson.M{
			"comments": bson.M{
				"created": time.Now().Format(time.RFC3339),
				"author":  bson.M{"username": user.Username, "_id": user.UserID},
				"body":    comment,
				"_id":     primitive.NewObjectID(),
			},
		}}
	if res, err := ps.posts.UpdateByID(ctx, hexPostID, update); err != nil {
		return err
	} else if res.MatchedCount != 1 || res.ModifiedCount != 1 {
		return errors.New("mongo: cannot make comment")
	}
	return nil
}

func (ps *PostStorageImpl) MakePost(newPost *NewPost, user User, ctx context.Context) (string, error) {
	post := &Post{
		Type:  newPost.Type,
		Title: newPost.Title,
		URL:   newPost.URL,
		Author: Author{
			Username: user.Username,
			ID:       user.UserID,
		},
		Votes:    []Vote{},
		Comments: []Comment{},
		Category: newPost.Category,
		Text:     newPost.Text,
		Created:  time.Now().Format(time.RFC3339),
	}
	res, err := ps.posts.InsertOne(ctx, post)
	if err != nil {
		return "", err
	}
	rawID, ok := res.InsertedID.(primitive.ObjectID)
	if !ok {
		return "", errors.New("cannot get raw postID of new post")
	}
	str := hex.EncodeToString(rawID[:])
	return str, nil
}

func (ps *PostStorageImpl) DeleteComment(postID, commentID string, ctx context.Context) error {
	// log.Printf("%s/%s\n", postID, commentID)
	hexPostID, err := primitive.ObjectIDFromHex(postID)
	if err != nil {
		return err
	}
	hexCommentID, err := primitive.ObjectIDFromHex(commentID)
	if err != nil {
		return err
	}
	filter := bson.M{"_id": hexPostID}
	update := bson.M{"$pull": bson.M{
		"comments": bson.M{
			"_id": bson.M{"$in": bson.A{hexCommentID}}},
	}}

	if res, err := ps.posts.UpdateOne(ctx, filter, update); err != nil {
		return err
	} else if res.ModifiedCount != 1 {
		return errors.New("cannot delete comment")
	}
	return nil
}

func (ps *PostStorageImpl) DeletePost(postID string, ctx context.Context) error {
	hexPostID, err := primitive.ObjectIDFromHex(postID)
	if err != nil {
		return err
	}
	if res, err := ps.posts.DeleteOne(ctx, bson.M{"_id": hexPostID}); err != nil {
		return err
	} else if res.DeletedCount != 1 {
		return errors.New("cannot delete post")
	}
	return nil
}

func (ps *PostStorageImpl) Rate(postID string, rating int, user User, ctx context.Context) error {
	post := &Post{}
	hexPostID, err := primitive.ObjectIDFromHex(postID)
	if err != nil {
		return err
	}
	res := ps.posts.FindOne(ctx, bson.M{"_id": hexPostID})
	if err := res.Decode(post); err != nil {
		return err
	}
	post.ID = nil // can't update id
	voteExist := false
	for i := range post.Votes {
		if post.Votes[i].ID == user.UserID {
			post.Score += rating - post.Votes[i].Vote
			post.Votes[i].Vote = rating
			voteExist = true
			break
		}
	}
	if !voteExist {
		post.Score += rating
		post.Votes = append(post.Votes, Vote{user.UserID, rating})
	}
	post.UpvotePercentage = int(float64(post.Score*len(post.Votes)) / float64(len(post.Votes)) * .5 * 100)
	if res, err := ps.posts.ReplaceOne(ctx, bson.M{"_id": hexPostID}, *post); err != nil {
		return err
	} else if res.MatchedCount != 1 || res.ModifiedCount != 1 {
		return errors.New("cannot replace post")
	}
	return nil
}

func (ps *PostStorageImpl) Unrate(postID string, user User, ctx context.Context) error {
	post := &Post{}
	hexPostID, err := primitive.ObjectIDFromHex(postID)
	if err != nil {
		return err
	}
	res := ps.posts.FindOne(ctx, bson.M{"_id": hexPostID})
	if err := res.Decode(post); err != nil {
		return err
	}
	post.ID = nil // can't update id
	log.Printf("%v; %s", post.Votes, user.UserID)
	for i := range post.Votes {
		if post.Votes[i].ID == user.UserID {
			post.Score -= post.Votes[i].Vote
			if i+1 < len(post.Votes) {
				post.Votes = append(post.Votes[:i], post.Votes[i+1:]...)
			} else {
				post.Votes = post.Votes[:i]
			}
			if len(post.Votes) > 0 {
				post.UpvotePercentage = int(float64(post.Score*len(post.Votes)) / float64(len(post.Votes)) * .5 * 100)
			}
			break
		}
	}

	if res, err := ps.posts.ReplaceOne(ctx, bson.M{"_id": hexPostID}, *post); err != nil {
		return err
	} else if res.MatchedCount != 1 || res.ModifiedCount != 1 {
		return errors.New("cannot replace post")
	}
	return nil
}
