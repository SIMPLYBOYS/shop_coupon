package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateCouponReservation(t *testing.T) {
	UserID := int32(10)

	reservation, err := testQueries.CreateCouponReservation(context.Background(), UserID)
	require.NoError(t, err)
	require.NotEmpty(t, reservation)

	require.Equal(t, UserID, reservation.UserID)
	require.NotZero(t, reservation.ID)
	require.NotZero(t, reservation.ReservedAt)
}

