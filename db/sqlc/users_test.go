package db

import (
	"context"
	"testing"

	u "github.com/SIMPLYBOYS/shopcoupon/util"
	"github.com/stretchr/testify/require"
)

func generateUserInfo(t *testing.T) (string, string) {
	t.Helper()
	name, err := u.RandomName()
	require.NoError(t, err)
	domain, err := u.RandomDomain()
	require.NoError(t, err)
	return name, name + domain
}

func TestCreateUser(t *testing.T) {
	name, email := generateUserInfo(t)
	arg := CreateUserParams{
		Username: name,
		Email:   email,
	}
	user, err := testQueries.CreateUser(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, user)

	require.Equal(t, arg.Username, user.Username)
	require.Equal(t, arg.Email, user.Email)
}

func TestGetUser(t *testing.T) {
	name, email := generateUserInfo(t)
	arg := CreateUserParams{
		Username: name,
		Email:   email,
	}
	user, err := testQueries.CreateUser(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, user)

	user2, err := testQueries.GetUser(context.Background(), user.ID)
	require.NoError(t, err)
	require.NotEmpty(t, user2)

	require.Equal(t, user.Username, user2.Username)
	require.Equal(t, user.Email, user2.Email)
}

func TestUpdateUser(t *testing.T) {
	name, email := generateUserInfo(t)
	arg := CreateUserParams{
		Username: name,
		Email:   email,
	}
	user, err := testQueries.CreateUser(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, user)

	newName, err := u.RandomName()
	require.NoError(t, err)
	arg2 := UpdateUserParams{
		ID: user.ID,
		Username: newName,
		Email: email,
	}
	user2, err := testQueries.UpdateUser(context.Background(), arg2)
	require.NoError(t, err)
	require.NotEmpty(t, user2)

	require.Equal(t, arg2.Username, user2.Username)
	require.Equal(t, arg2.Email, user2.Email)
}

func TestDeleteUser(t *testing.T) {
	name, email := generateUserInfo(t)
	arg := CreateUserParams{
		Username: name,
		Email:    email,
	}
	user, err := testQueries.CreateUser(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, user)

	err = testQueries.DeleteUser(context.Background(), user.ID)
	require.NoError(t, err)

	user2, err := testQueries.GetUser(context.Background(), user.ID)
	require.Error(t, err)
	require.Empty(t, user2)
}