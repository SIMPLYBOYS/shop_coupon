package api

import (
	"errors"
	"log"
	"net/http"

	db "github.com/SIMPLYBOYS/shopcoupon/db/sqlc"
	"github.com/gin-gonic/gin"
)

type getUserRequest struct {
	ID int `uri:"id" binding:"required,min=1"`
}

func (s *Server) getUser(ctx *gin.Context) {
	var req getUserRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	log.Printf("req: %v", req)

	// Authorization check: ensure user can only access their own data
	authUserID, exists := ctx.Get(AuthUserIDKey)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, errorResponse(errors.New("unauthorized")))
		return
	}

	if authUserID.(int32) != int32(req.ID) {
		ctx.JSON(http.StatusForbidden, errorResponse(errors.New("access denied: cannot access other user's data")))
		return
	}

	user, err := s.store.GetUser(ctx, int32(req.ID))

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, user)
}

type createUserRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
}

func (s *Server) createUser(ctx *gin.Context) {
	var req createUserRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	arg := db.CreateUserParams{
		Username: req.Username,
		Email:    req.Email,
	}
	user, err := s.store.CreateUser(ctx, arg)

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, user)
}
