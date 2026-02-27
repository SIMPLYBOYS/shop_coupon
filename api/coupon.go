package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type getCouponRequest struct {
	Code string `uri:"code" binding:"required,min=1"`
}

func (s *Server) getCoupon(ctx *gin.Context) {
	var req getCouponRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	// Get authenticated user ID
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		if errors.Is(err, errUnauthorized) {
			ctx.JSON(http.StatusUnauthorized, errorResponse(err))
		} else {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		}
		return
	}

	coupon, err := s.store.GetCoupon(ctx, req.Code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, errorResponse(err))
		} else {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		}
		return
	}

	// Authorization check: only coupon owner can view the coupon
	if !coupon.UserID.Valid || coupon.UserID.Int32 != userID {
		ctx.JSON(http.StatusForbidden, errorResponse(errAccessDenied))
		return
	}

	ctx.JSON(http.StatusOK, coupon)
}
