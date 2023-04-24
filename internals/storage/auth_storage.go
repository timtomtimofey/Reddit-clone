package storage

import (
	"bytes"
	"crypto/md5"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/dgrijalva/jwt-go"
	redis "github.com/gomodule/redigo/redis"
)

const (
	saltSize       = 8
	tokenFreshDays = 7
	StatusLoginOK  = iota
	StatusUserNotFound
	StatusWrongPassword
)

type AuthStorageImpl struct {
	users    *sql.DB
	sessions redis.Conn
	secret   []byte
}

func NewAuthStorage(users *sql.DB, sessions redis.Conn, secret []byte) AuthStorage {
	return &AuthStorageImpl{
		users:    users,
		sessions: sessions,
		secret:   secret,
	}
}

func makeHashedPass(password string) []byte {
	bufLen := saltSize
	if bufLen < binary.MaxVarintLen64 {
		bufLen = binary.MaxVarintLen64
	}
	salt := make([]byte, bufLen)
	binary.PutUvarint(salt, rand.Uint64())
	salt = salt[:saltSize]
	hashedPass := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32) // don't want to make all parameters global constant; parameters should be equal to those in checkPass
	return append(salt, hashedPass...)
}

func checkPass(password string, hashedPass []byte) bool {
	salt := hashedPass[:saltSize]
	hashedCandidate := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	hashedCandidate = append(salt, hashedCandidate...)
	return bytes.Equal(hashedCandidate, hashedPass)
}

func (as *AuthStorageImpl) CreateUser(username, password string) (string, error) {
	row := as.users.QueryRow("SELECT nextval(pg_get_serial_sequence('users', 'id'));")
	var rawID int64
	if err := row.Scan(&rawID); err != nil {
		return "", err
	}
	hash := md5.New()
	if err := binary.Write(hash, binary.LittleEndian, rawID); err != nil {
		return "", err
	}
	userID := hash.Sum(nil)
	hashedPass := makeHashedPass(password)
	if res, err := as.users.Exec("INSERT INTO users VALUES ($1,$2,$3,$4);", rawID, userID, username, hashedPass); err != nil {
		return "", err
	} else if aff, err := res.RowsAffected(); err != nil || aff == 0 {
		return "", errors.New("cannot create user")
	}
	return hex.EncodeToString(userID), nil
}

func (as *AuthStorageImpl) IsUserExist(username string) (bool, error) {
	row := as.users.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = $1);", username)
	var res bool
	if err := row.Scan(&res); err != nil {
		return false, err
	}
	return res, nil
}

func (as *AuthStorageImpl) GetUserID(username string) (string, error) {
	row := as.users.QueryRow("SELECT user_id FROM users WHERE username = $1;", username)
	res := new([]byte)
	if err := row.Scan(res); err != nil {
		return "", err
	}
	return hex.EncodeToString(*res), nil
}

func (as *AuthStorageImpl) CheckCredentials(username, password string) (int, error) {
	row := as.users.QueryRow("SELECT password FROM users WHERE username = $1;", username)
	hashedPass := new([]byte)
	if err := row.Scan(hashedPass); err == sql.ErrNoRows {
		return StatusUserNotFound, nil
	} else if err != nil {
		return 0, err
	}
	if !checkPass(password, *hashedPass) {
		return StatusWrongPassword, nil
	}
	return StatusLoginOK, nil
}

type SessionJWTClaims struct {
	User `json:"user"`
	jwt.StandardClaims
}

func (as *AuthStorageImpl) CreateToken(userID, username string) (string, error) {
	data := SessionJWTClaims{
		User: User{
			Username: username,
			UserID:   userID,
		},
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(tokenFreshDays * 24 * time.Hour).Unix(),
			IssuedAt:  time.Now().Unix(),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, data).SignedString(as.secret)
	if err != nil {
		return "", err
	}
	if reply, err := as.sessions.Do("SADD", fmt.Sprintf("user_id:%s", userID), token); err != nil {
		return "", err
	} else if aff, ok := reply.(int64); !ok || aff != 1 {
		return "", errors.New("bad type response from redis")
	}
	return token, nil
}

func (as *AuthStorageImpl) ValidateToken(token string) (User, error) {
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		method, ok := token.Method.(*jwt.SigningMethodHMAC)
		if !ok || method.Alg() != "HS256" {
			return nil, fmt.Errorf("bad sign method")
		}
		return as.secret, nil
	}
	payload := &SessionJWTClaims{}
	if _, err := jwt.ParseWithClaims(token, payload, keyFunc); err != nil {
		return User{}, err
	}
	if err := payload.Valid(); err != nil {
		return User{}, err
	}
	if reply, err := as.sessions.Do("SISMEMBER", fmt.Sprintf("user_id:%s", payload.UserID), token); err != nil {
		return User{}, err
	} else if present, ok := reply.(int64); !ok {
		return User{}, errors.New("bad type response from redis")
	} else if present == 0 {
		return User{}, errors.New("token not in db")
	}
	return payload.User, nil
}

// who will delete old tokens?
