package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"reddit_clone/internals/misc"
	"reddit_clone/internals/storage"
)

const (
	maxUsernameLen = 32
	minPassLen     = 8
)

type AuthHandler struct {
	Storage storage.AuthStorage
}

// aka credentials
type UserCred struct {
	Password string `json:"password"`
	Username string `json:"username"`
}

type Token struct {
	Token string `json:"token"`
}

func isPossibleCred(userCred *UserCred, errors *misc.ErrorBuilder) {
	if len(userCred.Username) == 0 {
		errors.Add("body", "username", "", "cannot be blank")
	} else if len(userCred.Username) > maxUsernameLen {
		errStr := fmt.Sprintf("must be at most %d characters long", maxUsernameLen)
		errors.Add("body", "username", userCred.Username, errStr)
	} else if misc.IsBorderSpace(userCred.Username) {
		errors.Add("body", "username", userCred.Username, "cannot start or end with whitespace")
	} else if !misc.IsValidUsername(userCred.Username) {
		errors.Add("body", "username", userCred.Username, "contains invalid characters")
	}

	if len(userCred.Password) == 0 {
		errors.Add("body", "password", userCred.Password, "cannot be blank")
	} else if len(userCred.Password) < minPassLen {
		errStr := fmt.Sprintf("must be at least %d characters long", minPassLen)
		errors.Add("body", "password", userCred.Password, errStr)
	}
}

func (ah *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	userCred := &UserCred{}
	if err := json.NewDecoder(r.Body).Decode(&userCred); err != nil {
		http.Error(w, misc.FormMessage("bad request"), http.StatusBadRequest)
		return
	}

	errors := misc.NewErrorBuilder()
	isPossibleCred(userCred, errors)
	if exist, err := ah.Storage.IsUserExist(userCred.Username); err != nil {
		log.Printf("handlers/auth.go: storage IsUserExist: %s\n", err)
		misc.InternalError(w)
		return
	} else if exist {
		errors.Add("body", "username", userCred.Username, "already exists")
	}
	if !errors.Empty() {
		http.Error(w, errors.Error(), http.StatusUnprocessableEntity)
		return
	}

	userID, err := ah.Storage.CreateUser(userCred.Username, userCred.Password)
	if err != nil {
		log.Printf("handlers/auth.go: storage CreateUser: %s\n", err)
		misc.InternalError(w)
		return
	}
	token, err := ah.Storage.CreateToken(userID, userCred.Username)
	if err != nil {
		log.Printf("handlers/auth.go: storage CreateToken: %s\n", err)
		misc.InternalError(w)
		return
	}
	tokenRaw, _ := json.Marshal(Token{Token: token}) // ignored error
	w.WriteHeader(http.StatusCreated)
	w.Write(tokenRaw)
}

func (ah *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	userCred := &UserCred{}
	if err := json.NewDecoder(r.Body).Decode(&userCred); err != nil {
		http.Error(w, misc.FormMessage("bad request"), http.StatusBadRequest)
		return
	}

	errors := misc.NewErrorBuilder()
	isPossibleCred(userCred, errors)
	if !errors.Empty() {
		http.Error(w, errors.Error(), http.StatusUnprocessableEntity)
		return
	}

	if respCode, err := ah.Storage.CheckCredentials(userCred.Username, userCred.Password); err != nil {
		log.Printf("handlers/auth.go: storage CheckCredentials: %s\n", err)
		misc.InternalError(w)
		return
	} else if respCode == storage.StatusUserNotFound {
		http.Error(w, misc.FormMessage("user not found"), http.StatusUnauthorized)
		return
	} else if respCode == storage.StatusWrongPassword {
		http.Error(w, misc.FormMessage("invalid password"), http.StatusUnauthorized)
		return
	} else if respCode != storage.StatusLoginOK {
		log.Printf("handlers/auth.go: storage CheckCredentials: %s\n", err)
		misc.InternalError(w)
		return
	}

	userID, err := ah.Storage.GetUserID(userCred.Username)
	if err != nil {
		log.Printf("handlers/auth.go: storage GetUserID: %s\n", err)
		misc.InternalError(w)
		return
	}
	token, err := ah.Storage.CreateToken(userID, userCred.Username)
	if err != nil {
		log.Printf("handlers/auth.go: storage CreateToken: %s\n", err)
		misc.InternalError(w)
		return
	}
	tokenRaw, _ := json.Marshal(Token{Token: token}) // ignored error
	w.Write(tokenRaw)
}

var (
	key = Token{"user"} // intended as a constat key to get storage.User from requests ctx
)

// Due to usage of gorilla/mux subrouting there is no need to check if authentification is needed
func (ah *AuthHandler) CheckAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authStr := r.Header.Get("Authorization")
		authParts := strings.Split(authStr, " ")
		if len(authParts) < 2 || authParts[0] != "Bearer" {
			http.Error(w, misc.FormMessage("unauthorized"), http.StatusUnauthorized)
			return
		}
		user, err := ah.Storage.ValidateToken(authParts[1])
		if err != nil {
			http.Error(w, misc.FormMessage("unauthorized"), http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), key, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetUser(r *http.Request) (storage.User, error) {
	user, ok := r.Context().Value(key).(storage.User)
	if !ok {
		return storage.User{}, errors.New("cannot get user from context")
	} else {
		return user, nil
	}
}
