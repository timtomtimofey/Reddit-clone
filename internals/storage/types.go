package storage

import (
	"context"
	"encoding/hex"
	"encoding/json"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
	Username string `json:"username"`
	UserID   string `json:"id"`
}

type AuthStorage interface {
	CreateUser(username, password string) (string, error)
	IsUserExist(username string) (bool, error)
	GetUserID(username string) (string, error)
	CheckCredentials(username, password string) (int, error)
	CreateToken(userID, username string) (string, error)
	ValidateToken(token string) (User, error)
}

type IDtype []byte

func (id *IDtype) MarshalBSON() ([]byte, error) {
	hex := hex.EncodeToString([]byte(*id))
	mongoID, err := primitive.ObjectIDFromHex(hex)
	if err != nil {
		return nil, err
	}
	_, data, err := bson.MarshalValue(mongoID)
	return data, err
}

func (id *IDtype) UnmarshalBSON(data []byte) error {
	*id = data
	return nil
}

func (id *IDtype) MarshalJSON() ([]byte, error) {
	hex := hex.EncodeToString([]byte(*id))
	return json.Marshal(hex)
}

func (id *IDtype) UnmarshalJSON(data []byte) error {
	slice, err := hex.DecodeString(string(data))
	(*id) = slice
	return err
}

type NewPost struct {
	Category string `json:"category"`
	Type     string `json:"type"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Text     string `json:"text"`
}

type Author struct {
	Username string `json:"username"`
	ID       string `json:"id" bson:"_id"`
}

type Comment struct {
	Created string `json:"created"`
	Author  Author `json:"author"`
	Body    string `json:"body"`
	ID      IDtype `json:"id" bson:"_id,omitempty"`
}

type Vote struct {
	ID   string `json:"id" bson:"_id"`
	Vote int    `json:"vote"`
}

type Post struct {
	Score            int       `json:"score"             bson:"score"`
	Views            int       `json:"views"             bson:"views"`
	Type             string    `json:"type"              bson:"type"`
	Title            string    `json:"title"             bson:"title"`
	URL              string    `json:"url"               bson:"url,omitempty"`
	Author           Author    `json:"author"            bson:"author"`
	Category         string    `json:"category"          bson:"category"`
	Text             string    `json:"text"              bson:"text,omitempty"`
	Votes            []Vote    `json:"votes"             bson:"votes"`
	Comments         []Comment `json:"comments"          bson:"comments"`
	Created          string    `json:"created"           bson:"created"`
	UpvotePercentage int       `json:"upvotepercentage"  bson:"upvotepercentage"`
	ID               IDtype    `json:"id"                bson:"_id,omitempty"`
}

type PostStorage interface {
	GetPost(postID string, ctx context.Context) (*Post, error)
	GetPosts(ctx context.Context) ([]*Post, error)
	GetPostsByCategory(category string, ctx context.Context) ([]*Post, error)
	GetPostsByUsername(username string, ctx context.Context) ([]*Post, error)

	CheckPostExist(postID string, ctx context.Context) (bool, error)
	CheckCommentExist(postID, commentID string, ctx context.Context) (int, error)
	CheckUserExist(username string, ctx context.Context) (bool, error)
	CheckPostOwner(postID string, user User, ctx context.Context) (bool, error)
	CheckCommentOwner(postID, commentID string, user User, ctx context.Context) (bool, error)

	MakeComment(postID, comment string, user User, ctx context.Context) error
	MakePost(newPost *NewPost, user User, ctx context.Context) (string, error)
	DeleteComment(postID, commentID string, ctx context.Context) error
	DeletePost(postID string, ctx context.Context) error

	Rate(postID string, rating int, user User, ctx context.Context) error
	Unrate(postID string, user User, ctx context.Context) error
}
