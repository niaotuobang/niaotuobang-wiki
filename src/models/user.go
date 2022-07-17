package models

import (
	"context"
	"log"

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
	log.Printf("query id %#v \n", args.ID)
	return &User{
		ID_:   args.ID,
		Name_: "John Doe",
	}, nil
}

// curl -X POST http://127.0.0.1:8080/query -d '{"query": "{ user(id:\"a\") { id, name } }"}'
