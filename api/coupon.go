package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type getCouponRequest struct {
	CODE string `uri:"code" binding:"required,min=1"`
}

func (s *Server) getCoupon(ctx *gin.Context) {
	var req getCouponRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	coupon, err := s.store.GetCoupon(ctx, req.CODE)

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, coupon)
}