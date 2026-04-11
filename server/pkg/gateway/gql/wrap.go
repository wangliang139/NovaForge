package gql

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type GqlError struct {
	Message    string         `json:"message"`
	Path       []string       `json:"path"`
	Extensions map[string]any `json:"extensions"`
}

type GqlResponse struct {
	Errors []GqlError `json:"errors"`
	Data   any        `json:"data"`
}

func WrapGinResponse(c *gin.Context, data any) {
	if c == nil {
		return
	}
	c.JSON(http.StatusOK, GqlResponse{
		Data: data,
	})
}

func WrapGinError(c *gin.Context, status int, message string) {
	if c == nil {
		return
	}
	if status == 0 {
		status = http.StatusInternalServerError
	}
	c.JSON(http.StatusOK, GqlResponse{
		Errors: []GqlError{
			{
				Message: message,
				Extensions: map[string]any{
					"code": status,
				},
			},
		},
	})
}
