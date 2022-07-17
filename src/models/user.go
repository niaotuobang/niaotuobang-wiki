package models

import (
	"context"

	"github.com/graph-gophers/graphql-go"
)

type UserArgs struct {
	ID string
}

type User struct {
	ID_   string
	Name_ string
}

func (u User) ID() graphql.ID {
	return graphql.ID(u.ID_)
}

func (u User) Name() string {
	return u.Name_
}

func (r *Query) User(ctx context.Context, args UserArgs) (*User, error) {
	return &User{
		ID_:   args.ID,
		Name_: "John Doe",
	}, nil
}
