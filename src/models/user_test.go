package models

import (
	"context"
	"testing"

	"github.com/graph-gophers/graphql-go"
)

func TestWithUser(t *testing.T) {
	var ctx = context.Background()
	schema := graphql.MustParseSchema(Schema, &Query{})

	const query = `query { user(id: "a") { id, name } }`
	response := schema.Exec(ctx, query, "", nil)
	const expect = `{"user":{"id":"a","name":"John Doe"}}`

	if string(response.Data) != expect {
		t.Errorf("response is %#v", string(response.Data))
	}
}
