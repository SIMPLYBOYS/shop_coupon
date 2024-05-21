package db

import (
	"context"
	"database/sql"
	"log"
	"testing"
	"time"

	u "github.com/SIMPLYBOYS/shopcoupon/util"
	"github.com/stretchr/testify/require"
)

// Import the package that defines the Coupon type

func TestCreateCoupon(t *testing.T) {

	arg := CreateCouponParams{
		Code:    u.GenerateCouponCode(),
		Discount: "0.25",
		ExpiryDate:  time.Now(),
	}
	coupon, err := testQueries.CreateCoupon(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, coupon)

	require.Equal(t, arg.Code, coupon.Code)
	require.Equal(t, arg.Discount, coupon.Discount)
}

func TestGeCoupon(t *testing.T) {
	arg := CreateCouponParams{
		Code:    u.GenerateCouponCode(),
		Discount: "0.25",
		ExpiryDate:  time.Now(),
	}
	coupon, err := testQueries.CreateCoupon(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, coupon)

	coupon2, err := testQueries.GetCoupon(context.Background(), coupon.Code)
	require.NoError(t, err)
	require.NotEmpty(t, coupon2)

	require.Equal(t, coupon.Code, coupon2.Code)
	require.Equal(t, coupon.Discount, coupon2.Discount)
}

func TestGetList(t *testing.T) {
	args := []CreateCouponParams{
		{
			Code:       u.GenerateCouponCode(),
			Discount:   "0.25",
			ExpiryDate: time.Now(),
		},
		{
			Code:       u.GenerateCouponCode(),
			Discount:   "0.15",
			ExpiryDate: time.Now(),
		},
	}


	for _, arg := range args {
		coupon, err := testQueries.CreateCoupon(context.Background(), arg)
		require.NoError(t, err)
		require.NotEmpty(t, coupon)
	}

	coupons, err := testQueries.ListAvailableCoupons(context.Background(), time.Now())
	require.NoError(t, err)
	require.NotEmpty(t, coupons)

	for i, coupon := range coupons {
		require.Equal(t, args[i].Code, coupon.Code)
		require.Equal(t, args[i].Discount, coupon.Discount)
	}
}

func TestUpdateCoupon(t *testing.T) {
	arg := CreateCouponParams{
		Code:    u.GenerateCouponCode(),
		Discount: "0.25",
		ExpiryDate:  time.Now(),
	}
	coupon, err := testQueries.CreateCoupon(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, coupon)

	log.Print("coupon:", coupon.UserID)

	// Assuming you have a valid userId integer value
	userId := 1

	// Create a non-null sql.NullInt32 value
	nullableUserId := sql.NullInt32{
	    Int32: int32(userId),
	    Valid: true,
	}


	arg2 := UpdateCouponParams{
		Code:   coupon.Code,
		ID: 	 coupon.ID,
		UserID: nullableUserId,
		ExpiryDate: time.Now().AddDate(0, 0, 7),
		Discount:   "0.15",
	}
	coupon, err = testQueries.UpdateCoupon(context.Background(), arg2)
	require.NoError(t, err)
	require.NotEmpty(t, coupon)

	require.Equal(t, arg2.Discount, coupon.Discount)
}