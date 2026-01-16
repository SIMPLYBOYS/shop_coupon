package api

import (
	"errors"
	"log"
	"net/http"

	db "github.com/SIMPLYBOYS/shopcoupon/db/sqlc"
	"github.com/gin-gonic/gin"
)

type getUserRequest struct {
	ID int32 `uri:"id" binding:"required,min=1"`
}

func (s *Server) getUser(ctx *gin.Context) {
	var req getUserRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	// Authorization check: ensure user can only access their own data
	authUserID, exists := ctx.Get(AuthUserIDKey)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, errorResponse(errors.New("unauthorized")))
		return
	}

	userID, ok := authUserID.(int32)
	if !ok {
		log.Printf("ERROR: invalid user ID type in context: %T", authUserID)
		ctx.JSON(http.StatusInternalServerError, errorResponse(errors.New("internal server error")))
		return
	}

	log.Printf("getUser request: id=%d, authUserID=%d", req.ID, userID)

	if userID != req.ID {
		ctx.JSON(http.StatusForbidden, errorResponse(errors.New("access denied")))
		return
	}

	user, err := s.store.GetUser(ctx, req.ID)

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
