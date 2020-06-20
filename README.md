# GoVert 
A helper function generator to convert your models to your domain types easier.

GoVert is a fork of [gqlgen-sqlboiler](https://github.com/web-ridge/gqlgen-sqlboiler), an awesome file generator for SQL Boil and GraphQL. This library focuses less on the GraphQL side and more on the helper function generation.

Features:
- Auto generate functions that map your domain types to your models, and back again. Stop writing the same boilerplate code, over and over again.
- Custom return / domain types. Have 
- Custom primary keys. Generate custom functions that map builtin types (id, uint, string, etc.) or custom struct types to a number of utilities.
- Go.mod support