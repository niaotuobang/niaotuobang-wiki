package cmd

import (
	"log"
	"net/http"

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
	"github.com/niaotuobang/niaotuobang-wiki/src/models"
)

func WebMain() {
	schema := graphql.MustParseSchema(models.Schema, &models.Query{})
	http.Handle("/query", &relay.Handler{Schema: schema})
	log.Fatal(http.ListenAndServe(":8080", nil))
}
