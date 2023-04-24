package misc

import (
	"encoding/json"
	"net/http"
)

func FormMessage(message string) string {
	data, _ := json.Marshal(struct {
		Message string `json:"message"`
	}{
		Message: message,
	})
	return string(data)
}

func InternalError(w http.ResponseWriter) {
	http.Error(w, FormMessage("internal error"), http.StatusInternalServerError)
}

type Error struct {
	Location string `json:"location"`
	Param    string `json:"param"`
	Value    string `json:"value"`
	Msg      string `json:"msg"`
}

func FormError(location, param, value, msg string) string {
	data, _ := json.Marshal(struct {
		Errors []Error `json:"errors"`
	}{
		Errors: []Error{
			{
				Location: location,
				Param:    param,
				Value:    value,
				Msg:      msg,
			},
		},
	})
	return string(data)
}

type ErrorBuilder struct {
	errors []Error
}

func NewErrorBuilder() *ErrorBuilder {
	return &ErrorBuilder{
		errors: make([]Error, 0, 10),
	}
}

func (eb *ErrorBuilder) Add(location, param, value, msg string) {
	eb.errors = append(eb.errors, Error{location, param, value, msg})
}

func (eb *ErrorBuilder) Empty() bool {
	return len(eb.errors) == 0
}

func (eb *ErrorBuilder) Error() string {
	data, _ := json.Marshal(struct {
		Errors []Error `json:"errors"`
	}{
		Errors: eb.errors,
	})
	return string(data)
}
