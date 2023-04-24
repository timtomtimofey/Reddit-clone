package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"path"

	"reddit_clone/internals/misc"
	"reddit_clone/internals/storage"

	"github.com/gorilla/mux"
)

type PostHandler struct {
	Storage storage.PostStorage
}

//-------------------------------------Get Post & Posts--------------------------------//

// implicit add view in Storage.GetPost
func (ph *PostHandler) GetPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	postID, ok := mux.Vars(r)["post_id"]
	if !ok {
		log.Printf("handlers/posts.go: GetPost: bad routing: %s\n", r.URL.Path)
		misc.InternalError(w)
		return
	}
	if exist, err := ph.Storage.CheckPostExist(postID, ctx); err != nil {
		log.Printf("handlers/posts.go: GetPost: cannot check post existance: %s\n", err)
		misc.InternalError(w)
		return
	} else if !exist {
		http.Error(w, misc.FormMessage("post not found"), http.StatusNotFound)
		return
	}
	data, err := ph.Storage.GetPost(postID, ctx)
	if err != nil {
		log.Printf("handlers/posts.go: GetPost: cannot get post: %s\n", err)
		misc.InternalError(w)
		return
	}
	dataRaw, _ := json.Marshal(data)
	w.Write(dataRaw)
}

func (ph *PostHandler) GetPosts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data, err := ph.Storage.GetPosts(ctx)
	if err != nil {
		log.Printf("handlers/posts.go: GetPosts: cannot get posts: %s\n", err)
		misc.InternalError(w)
		return
	}
	dataRaw, _ := json.Marshal(data)
	w.Write(dataRaw)
}

func (ph *PostHandler) GetPostsByCategory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	category, ok := mux.Vars(r)["category"]
	if !ok {
		log.Printf("handlers/posts.go: GetPostsByCategory: bad routing: %s\n", r.URL.Path)
		misc.InternalError(w)
		http.Error(w, misc.FormMessage("internal error: bad routing"), http.StatusInternalServerError)
		return
	}
	data, err := ph.Storage.GetPostsByCategory(category, ctx)
	if err != nil {
		log.Printf("handlers/posts.go: GetPostsByCategory: cannot get posts: %s\n", err)
		misc.InternalError(w)
		return
	}
	dataRaw, _ := json.Marshal(data)
	w.Write(dataRaw)
}

func (ph *PostHandler) GetPostByUsername(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username, ok := mux.Vars(r)["username"]
	if !ok {
		log.Printf("handlers/posts.go: GetPostsByUsername: bad routing: %s\n", r.URL.Path)
		misc.InternalError(w)
		return
	}
	if exist, err := ph.Storage.CheckUserExist(username, ctx); err != nil {
		log.Printf("handlers/posts.go: GetPostsByUsername: cannot check user existance: %s\n", err)
		misc.InternalError(w)
		return
	} else if !exist {
		http.Error(w, misc.FormMessage("user not exist"), http.StatusNotFound)
		return
	}
	data, err := ph.Storage.GetPostsByUsername(username, ctx)
	if err != nil {
		log.Printf("handlers/posts.go: GetPostsByUsername: cannot get posts by user: %s\n", err)
		misc.InternalError(w)
		return
	}
	dataRaw, _ := json.Marshal(data)
	w.Write(dataRaw)
}

//-------------------------------------Create and delete post--------------------------//

func (ph *PostHandler) MakePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	minPostLen := 4
	validTypes := map[string]struct{}{
		"text": {},
		"link": {},
	}

	post := &storage.NewPost{}
	if err := json.NewDecoder(r.Body).Decode(post); err != nil {
		http.Error(w, misc.FormMessage("bad request"), http.StatusBadRequest)
		return
	}
	if post.Type == "link" {
		post.Text = ""
	} else if post.Type == "text" {
		post.URL = ""
	}

	errors := misc.NewErrorBuilder()

	// title
	if len(post.Title) == 0 {
		errors.Add("body", "title", "", "cannot be blank")
	} else if misc.IsBorderSpace(post.Title) {
		errors.Add("body", "title", post.Title, "cannot start or end with whitespace")
	}

	// url or text depending on type value
	isValidURL := func(str string) bool {
		u, err := url.Parse(str)
		return err == nil && u.Scheme != "" && u.Host != ""
	}
	if post.Type == "link" && !isValidURL(post.URL) {
		errors.Add("body", "url", post.URL, "is invalid")
	} else if post.Type != "link" && len(post.Text) < minPostLen {
		errors.Add("body", "text", post.Text, "must be at least 4 characters long")
	}

	// category
	if len(post.Category) == 0 {
		errors.Add("body", "category", "", "cannot be blank")
	}

	// type
	if _, ok := validTypes[post.Type]; !ok {
		errors.Add("body", "type", post.Type, "must be a link or text post")
	}

	if !errors.Empty() {
		http.Error(w, errors.Error(), http.StatusUnprocessableEntity)
		return
	}

	user, err := GetUser(r)
	if err != nil {
		log.Printf("handlers/posts.go: MakePost: cannot get user: %s\n", err)
		misc.InternalError(w)
		return
	}
	postID, err := ph.Storage.MakePost(post, user, ctx)
	if err != nil {
		log.Printf("handlers/posts.go: MakePost: cannot make post: %s\n", err)
		misc.InternalError(w)
		return
	}
	if err := ph.Storage.Rate(postID, 1, user, ctx); err != nil {
		log.Printf("handlers/posts.go: MakePost: cannot upvote new post: %s\n", err)
		misc.InternalError(w)
		return
	}

	data, err := ph.Storage.GetPost(postID, ctx)
	if err != nil {
		log.Printf("handlers/posts.go: MakePost: cannot get post: %s\n", err)
		misc.InternalError(w)
		return
	}
	dataRaw, _ := json.Marshal(data)
	w.WriteHeader(http.StatusCreated)
	w.Write(dataRaw)
}

func (ph *PostHandler) DeletePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	postID, ok := mux.Vars(r)["post_id"]
	if !ok {
		log.Printf("handlers/posts.go: DeletePost: bad routing: %s\n", r.URL.Path)
		misc.InternalError(w)
		return
	} else if exist, err := ph.Storage.CheckPostExist(postID, ctx); err != nil {
		log.Printf("handlers/posts.go: DeletePost: cannot check post existance: %s\n", err)
		misc.InternalError(w)
		return
	} else if !exist {
		http.Error(w, misc.FormMessage("post not found"), http.StatusNotFound)
		return
	}

	if user, err := GetUser(r); err != nil {
		log.Printf("handlers/posts.go: DeletePost: cannot get user: %s\n", err)
		misc.InternalError(w)
		return
	} else if isAuthorized, err := ph.Storage.CheckPostOwner(postID, user, ctx); err != nil {
		log.Printf("handlers/posts.go: DeletePost: cannot check post owner: %s\n", err)
		misc.InternalError(w)
		return
	} else if !isAuthorized {
		http.Error(w, misc.FormMessage("unauthorized"), http.StatusUnauthorized)
		return
	}

	if err := ph.Storage.DeletePost(postID, ctx); err != nil {
		log.Printf("handlers/posts.go: DeletePost: cannot delete post: %s\n", err)
		misc.InternalError(w)
		return
	}
	w.Write([]byte(misc.FormMessage("success")))
}

//-----------------------------Create and delete comments------------------------------//

type Comment struct {
	Comment string `json:"comment"`
}

func (ph *PostHandler) MakeComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	postID, ok := mux.Vars(r)["post_id"]
	if !ok {
		log.Printf("handlers/posts.go: MakeComment: bad routing: %s\n", r.URL.Path)
		misc.InternalError(w)
		return
	}
	if exist, err := ph.Storage.CheckPostExist(postID, ctx); err != nil {
		log.Printf("handlers/posts.go: MakeComment: cannot check post existance: %s\n", err)
		misc.InternalError(w)
		return
	} else if !exist {
		http.Error(w, misc.FormMessage("post not found"), http.StatusNotFound)
		return
	}
	comment := &Comment{}
	if err := json.NewDecoder(r.Body).Decode(comment); err != nil {
		http.Error(w, misc.FormMessage("bad request"), http.StatusBadRequest)
		return
	} else if len(comment.Comment) == 0 {
		http.Error(w, misc.FormError("body", "comment", "", "is required"), http.StatusUnprocessableEntity)
		return
	}
	user, err := GetUser(r)
	if err != nil {
		log.Printf("handlers/posts.go: MakeComment: cannot get user: %s\n", err)
		misc.InternalError(w)
		return
	}
	if err := ph.Storage.MakeComment(postID, comment.Comment, user, ctx); err != nil {
		log.Printf("handlers/posts.go: MakeComment: cannot make comment: %s\n", err)
		misc.InternalError(w)
		return
	}
	data, err := ph.Storage.GetPost(postID, ctx)
	if err != nil {
		log.Printf("handlers/posts.go: MakeComment: cannot get post: %s\n", err)
		misc.InternalError(w)
		return
	}
	dataRaw, _ := json.Marshal(data)
	w.WriteHeader(http.StatusCreated)
	w.Write(dataRaw)
}

func (ph *PostHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	postID, postOK := mux.Vars(r)["post_id"]
	commentID, commentOK := mux.Vars(r)["comment_id"]
	if !postOK || !commentOK {
		log.Printf("handlers/posts.go: DeleteComment: cannot check post existance: %s\n", r.URL.Path)
		misc.InternalError(w)
		return
	}
	if respCode, err := ph.Storage.CheckCommentExist(postID, commentID, ctx); err != nil {
		log.Printf("handlers/posts.go: DeleteComment: cannot check comment existance: %s\n", err)
		misc.InternalError(w)
		return
	} else if respCode == storage.StatusPostNotExist {
		http.Error(w, misc.FormMessage("post not found"), http.StatusNotFound)
		return
	} else if respCode == storage.StatusCommentNotExist {
		http.Error(w, misc.FormMessage("comment not found"), http.StatusNotFound)
		return
	} else if respCode != storage.StatusCommentOK {
		log.Printf("handlers/posts.go: DeleteComment: unknown respCode: %d\n", respCode)
		misc.InternalError(w)
		return
	}

	user, err := GetUser(r)
	if err != nil {
		log.Printf("handlers/posts.go: DeleteComment: cannot get user: %s\n", err)
		misc.InternalError(w)
		return
	}
	if isAuthorized, err := ph.Storage.CheckCommentOwner(postID, commentID, user, ctx); err != nil {
		log.Printf("handlers/posts.go: DeleteComment: cannot check comment owner: %s\n", err)
		misc.InternalError(w)
		return
	} else if !isAuthorized {
		http.Error(w, misc.FormMessage("unauthorized"), http.StatusUnauthorized)
		return
	}

	if err := ph.Storage.DeleteComment(postID, commentID, ctx); err != nil {
		log.Printf("handlers/posts.go: DeleteComment: cannot delete comment: %s\n", err)
		misc.InternalError(w)
		return
	}

	data, err := ph.Storage.GetPost(postID, ctx)
	if err != nil {
		log.Printf("handlers/posts.go: DeleteComment: cannot get post: %s\n", err)
		misc.InternalError(w)
		return
	}
	dataRaw, _ := json.Marshal(data)
	w.WriteHeader(http.StatusCreated)
	w.Write(dataRaw)
}

//---------------------------------------Rating----------------------------------------//

func (ph *PostHandler) Vote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	postID, ok := mux.Vars(r)["post_id"]
	if !ok {
		log.Printf("handlers/posts.go: Vote: bad routing: %s\n", r.URL.Path)
		misc.InternalError(w)
		return
	} else if exist, err := ph.Storage.CheckPostExist(postID, ctx); err != nil {
		log.Printf("handlers/posts.go: Vote: cannot check post existance: %s\n", err)
		misc.InternalError(w)
		return
	} else if !exist {
		http.Error(w, misc.FormMessage("post not found"), http.StatusNotFound)
		return
	}

	user, err := GetUser(r)
	if err != nil {
		log.Printf("handlers/posts.go: Vote: cannot get user: %s\n", err)
		misc.InternalError(w)
		return
	}
	switch path.Base(r.URL.Path) {
	case "upvote":
		err = ph.Storage.Rate(postID, 1, user, ctx)
	case "downvote":
		err = ph.Storage.Rate(postID, -1, user, ctx)
	case "unvote":
		err = ph.Storage.Unrate(postID, user, ctx)
	}
	if err != nil {
		log.Printf("handlers/posts.go: Vote: cannot vote: %s\n", err)
		misc.InternalError(w)
		return
	}

	data, err := ph.Storage.GetPost(postID, ctx)
	if err != nil {
		log.Printf("handlers/posts.go: Vote: cannot get post: %s\n", err)
		misc.InternalError(w)
		return
	}
	dataRaw, _ := json.Marshal(data)
	w.Write(dataRaw)
}
