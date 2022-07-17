package models

const Schema = `
schema {
	query: Query
}

type Query {
	user(id: ID!): User!
}

type User {
	id: ID!
	name: String!
}
`

type Query struct{}
