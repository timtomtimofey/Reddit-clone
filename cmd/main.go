package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"reddit_clone/internals/handlers"
	"reddit_clone/internals/storage"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"

	"github.com/gomodule/redigo/redis"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func main() {
	usersDB, err := sql.Open("postgres", "postgres://root:toor@localhost:5432/reddit_clone?sslmode=disable")
	if err != nil {
		log.Fatalf("main.go sql.Open: %s\n", err)
	}
	usersDB.SetMaxOpenConns(10)
	if err := usersDB.Ping(); err != nil {
		log.Fatalf("main.go postgres Ping: %s\n", err)
	} else {
		fmt.Printf("Connected to PostgreSQL\n")
	}

	ctx := context.Background()
	opts := options.Client().ApplyURI("mongodb://localhost:27017")
	mongoConn, err := mongo.NewClient(opts)
	if err != nil {
		log.Fatalf("main.go mongo.NewCient: %s\n", err)
	} else if err = mongoConn.Connect(ctx); err != nil {
		log.Fatalf("main.go mongo Connect: %s\n", err)
	} else if err = mongoConn.Ping(ctx, readpref.Primary()); err != nil {
		log.Fatalf("main.go: mongo Ping: %s\n", err)
	} else {
		fmt.Printf("Connected to MongoDB\n")
	}
	defer func() {
		if err = mongoConn.Disconnect(ctx); err != nil {
			log.Fatalf("main.go: mongo Disconnect: %s\n", err)
		}
	}()
	messagesConn := mongoConn.Database("reddit_clone").Collection("messages")

	sessionsConn, err := redis.DialURL("redis://user:@localhost:6379/0")
	if err != nil {
		log.Fatalf("main.go: redis.DialURL: %s", err)
	} else {
		fmt.Printf("Connected to Redis\n")
	}

	authStorage := storage.NewAuthStorage(usersDB, sessionsConn, []byte{1, 2, 3})
	postStorage := storage.NewPostStorage(messagesConn, usersDB)

	authHandler := handlers.AuthHandler{Storage: authStorage}
	postHandler := handlers.PostHandler{Storage: postStorage}

	mux := mux.NewRouter()
	fileServer := http.FileServer(http.Dir("./template"))
	mux.Handle("/", fileServer)
	mux.PathPrefix("/static/").Handler(fileServer)

	mux.HandleFunc("/api/register", authHandler.Register)
	mux.HandleFunc("/api/login", authHandler.Login)

	mux.HandleFunc("/api/posts/", postHandler.GetPosts).Methods("GET")
	mux.HandleFunc("/api/post/{post_id:[0-9a-f]+}", postHandler.GetPost).Methods("GET")
	mux.HandleFunc("/api/posts/{category}", postHandler.GetPostsByCategory)
	mux.HandleFunc("/api/user/{username}", postHandler.GetPostByUsername)

	authMux := mux.PathPrefix("/").Subrouter() // everything under this subrouter need authentification and will be checked by authHandler.CheckAuth
	authMux.HandleFunc("/api/posts", postHandler.MakePost).Methods("POST")
	authMux.HandleFunc("/api/post/{post_id:[0-9a-f]+}", postHandler.DeletePost).Methods("DELETE")

	authMux.HandleFunc("/api/post/{post_id:[0-9a-f]+}", postHandler.MakeComment).Methods("POST")
	authMux.HandleFunc("/api/post/{post_id:[0-9a-f]+}/{comment_id:[0-9a-f]+}", postHandler.DeleteComment)

	authMux.HandleFunc("/api/post/{post_id:[0-9a-f]+}/upvote", postHandler.Vote)
	authMux.HandleFunc("/api/post/{post_id:[0-9a-f]+}/downvote", postHandler.Vote)
	authMux.HandleFunc("/api/post/{post_id:[0-9a-f]+}/unvote", postHandler.Vote)

	mux.Use(handlers.SetDate)
	authMux.Use(authHandler.CheckAuth)

	serv := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	fmt.Printf("Start server on %v\n", serv.Addr)
	serv.ListenAndServe()
}
